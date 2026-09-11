package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent/skills"
	"github.com/Napam/chatwego/lib/agent/tools"
	"github.com/openai/openai-go/v3"
)

// systemPromptTemplate lists each loaded skill (name + description) so the
// model knows what's available and can fetch one with the "skill" tool --
// only summaries, not full bodies. Data: skills.Sorted(). {{if .}} treats
// empty slice as false, so whole section incl. leading blank line disappears
// if no skills loaded.
var systemPromptTemplate = template.Must(template.New("systemPrompt").Parse(
	`
You are a helpful assistant.{{if .}}

Available skills. Fetch one with the skill tool before acting on it:
{{range .}}- {{.Name}}: {{.Description}}
{{end}}{{end}}
`,
))

// systemPrompt: rendered once at package init.
var systemPrompt = buildSystemPrompt()

func buildSystemPrompt() string {
	var b strings.Builder
	if err := systemPromptTemplate.Execute(&b, skills.Sorted()); err != nil {
		panic("failed to build system prompt: " + err.Error())
	}
	return b.String()
}

// maxToolRounds caps tool calls per reply. Past it, next round omits Tools,
// guaranteeing termination: a tool-less request can never return tool calls.
// Dropping Tools alone isn't enough for models like local llama.cpp that
// emulate tool calls via learned text tokens (no API grammar constraints) --
// they still emit that syntax as plain content. So stopCallingToolsNotice is
// appended to make the model stand down and answer directly.
const maxToolRounds = 25

// stopCallingToolsNotice: one-off system message appended on the
// maxToolRounds round. Request-only, never persisted to chat_messages, so it
// doesn't leak into future turns.
const stopCallingToolsNotice = "You have used the maximum number of tool calls " +
	"allowed for this reply. Do not attempt to call any more tools, and do not " +
	"output tool call syntax. Answer the user directly now, in plain text, " +
	"using the information already gathered."

// toolParams: Tools sent with every request, built from tools.All.
var toolParams = func() []openai.ChatCompletionToolUnionParam {
	params := make([]openai.ChatCompletionToolUnionParam, len(tools.All))
	for i, t := range tools.All {
		params[i] = t.Param()
	}
	return params
}()

// callTool runs a tool by name; result fed back to the model.
func callTool(name, arguments string) (string, error) {
	t, ok := tools.Find(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %q", name)
	}
	return t.Call(arguments)
}

// StoredToolCall: JSON shape in chat_messages.tool_calls; read back by
// buildMessages to reconstruct history.
type StoredToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallRecord: StoredToolCall + result, so callers can persist the full
// tool exchange (call + result) across turns.
type ToolCallRecord struct {
	StoredToolCall
	Result string
}

// Round: one request/response round. ToolCalls populated if it called tools,
// empty if it produced the final text reply.
type Round struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCallRecord
}

// toolCallParam: param sending a tool call back to the model; shared by live
// rounds and history reconstruction (StoredToolCall from the database).
func toolCallParam(tc StoredToolCall) openai.ChatCompletionMessageToolCallUnionParam {
	return openai.ChatCompletionMessageToolCallUnionParam{
		OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
			ID: tc.ID,
			Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		},
	}
}

// Agent streams chat replies from a session's message history via an
// OpenAI-compatible chat completion API.
type Agent struct {
	client openai.Client
	model  string
}

func New(client openai.Client, model string) *Agent {
	return &Agent{client: client, model: model}
}

// ReplyHandlers receives the events StreamReply emits. Every field is
// optional (nil handler skipped). Named rather than positional so OnDelta and
// OnReasoningDelta -- same signature -- can't be swapped by mistake. A
// handler returning an error aborts the reply.
type ReplyHandlers struct {
	// OnRoundStart fires before each round streams; caller sets up a fresh
	// destination (e.g. new chat bubble) per round, matching history
	// reconstruction on reload.
	OnRoundStart func() error
	// OnDelta fires per chunk of reply text.
	OnDelta func(delta string) error
	// OnReasoningDelta fires per chunk of reasoning text, if the model
	// exposes any (e.g. llama.cpp's "reasoning_content").
	OnReasoningDelta func(delta string) error
	// OnToolCalls fires once per round that called tools, with all calls and
	// results. Tools run synchronously, so nothing to stream.
	OnToolCalls func(calls []ToolCallRecord) error
}

// StreamReply sends session history to the LLM and streams the reply through
// h (see ReplyHandlers). Each round (see Round) is a separate request/
// response round-trip, persisted as its own row. Returns every round --
// including tool-calling ones -- so the caller can persist the full exchange,
// plus an error if the stream failed or a handler errored.
//
// Tool calls run locally; results feed back for another round. After
// maxToolRounds, tools are dropped and stopCallingToolsNotice is sent,
// forcing a text answer instead of cutting off mid-tool-call.
func (a *Agent) StreamReply(
	ctx context.Context,
	history []sqlc.ChatMessage,
	h ReplyHandlers,
) (rounds []Round, err error) {
	messages := buildMessages(history)

	for round := 0; ; round++ {
		if h.OnRoundStart != nil {
			if err := h.OnRoundStart(); err != nil {
				return rounds, fmt.Errorf("start round: %w", err)
			}
		}
		// Past the round cap: drop Tools and send stopCallingToolsNotice.
		// Both needed -- models like llama.cpp still emit tool-call syntax
		// as raw content otherwise.
		requestTools := toolParams
		requestMessages := messages
		if round >= maxToolRounds {
			requestTools = nil
			requestMessages = append(
				append([]openai.ChatCompletionMessageParamUnion{}, messages...),
				openai.SystemMessage(stopCallingToolsNotice),
			)
		}
		acc, text, reasoning, err := a.streamOnce(ctx, requestMessages, requestTools, h)
		if err != nil {
			rounds = append(rounds, Round{Content: text, Reasoning: reasoning})
			return rounds, err
		}

		if len(acc.Choices) == 0 {
			rounds = append(rounds, Round{Content: text, Reasoning: reasoning})
			return rounds, nil
		}
		toolCalls := acc.Choices[0].Message.ToolCalls
		if len(toolCalls) == 0 {
			rounds = append(rounds, Round{Content: text, Reasoning: reasoning})
			return rounds, nil
		}

		assistantMsg := openai.ChatCompletionAssistantMessageParam{
			Content: openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(text),
			},
		}
		records := make([]ToolCallRecord, 0, len(toolCalls))
		for _, toolCall := range toolCalls {
			// toolCall.ToParam() round-trips raw JSON, which the accumulator
			// never populates (it builds fields from deltas), so it marshals
			// empty. Build the param from accumulated fields instead.
			stored := StoredToolCall{
				ID:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			}
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, toolCallParam(stored))

			result, callErr := callTool(stored.Name, stored.Arguments)
			if callErr != nil {
				result = fmt.Sprintf("error: %s", callErr)
			}
			records = append(records, ToolCallRecord{StoredToolCall: stored, Result: result})
		}
		// Assistant's tool-call message must precede the tool results.
		messages = append(
			messages,
			openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg},
		)
		for _, record := range records {
			messages = append(messages, openai.ToolMessage(record.Result, record.ID))
		}
		if h.OnToolCalls != nil {
			if err := h.OnToolCalls(records); err != nil {
				rounds = append(
					rounds,
					Round{Content: text, Reasoning: reasoning, ToolCalls: records},
				)
				return rounds, fmt.Errorf("write tool calls: %w", err)
			}
		}
		rounds = append(rounds, Round{Content: text, Reasoning: reasoning, ToolCalls: records})
	}
}

// streamOnce sends one chat completion request and streams its reply;
// returns the accumulated completion (used to detect tool calls) plus the
// response text and reasoning text.
func (a *Agent) streamOnce(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	requestTools []openai.ChatCompletionToolUnionParam,
	h ReplyHandlers,
) (acc openai.ChatCompletionAccumulator, response, reasoning string, err error) {
	stream := a.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(a.model),
		Messages: messages,
		Tools:    requestTools,
	})
	defer func() {
		if closeErr := stream.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	var b, r strings.Builder
	for stream.Next() {
		if ctx.Err() != nil {
			return acc, b.String(), r.String(), ctx.Err()
		}
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if reasoning := reasoningDelta(delta); reasoning != "" {
			r.WriteString(reasoning)
			if h.OnReasoningDelta != nil {
				if err := h.OnReasoningDelta(reasoning); err != nil {
					return acc, b.String(), r.String(), fmt.Errorf("write reasoning delta: %w", err)
				}
			}
		}

		if delta.Content == "" {
			continue
		}
		b.WriteString(delta.Content)
		if h.OnDelta != nil {
			if err := h.OnDelta(delta.Content); err != nil {
				return acc, b.String(), r.String(), fmt.Errorf("write delta: %w", err)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return acc, b.String(), r.String(), fmt.Errorf("stream chat completion: %w", err)
	}
	return acc, b.String(), r.String(), nil
}

// reasoningDelta extracts "reasoning_content", streamed by llama.cpp (and
// other OpenAI-compatible servers) for reasoning models. Not part of the
// official schema, so it lands in JSON.ExtraFields, never the typed fields.
func reasoningDelta(delta openai.ChatCompletionChunkChoiceDelta) string {
	field, ok := delta.JSON.ExtraFields["reasoning_content"]
	if !ok {
		return ""
	}
	var content string
	if err := json.Unmarshal([]byte(field.Raw()), &content); err != nil {
		return ""
	}
	return content
}

func buildMessages(history []sqlc.ChatMessage) []openai.ChatCompletionMessageParamUnion {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+1)
	messages = append(messages, openai.SystemMessage(systemPrompt))
	for _, m := range history {
		switch m.Type {
		case "user":
			messages = append(messages, openai.UserMessage(m.Content.String))
		case "assistant":
			assistantMsg := openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(m.Content.String),
				},
			}
			if m.ToolCalls.Valid {
				var stored []StoredToolCall
				if err := json.Unmarshal([]byte(m.ToolCalls.String), &stored); err == nil {
					for _, tc := range stored {
						assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, toolCallParam(tc))
					}
				}
			}
			messages = append(
				messages,
				openai.ChatCompletionMessageParamUnion{OfAssistant: &assistantMsg},
			)
		case "tool":
			messages = append(messages, openai.ToolMessage(m.Content.String, m.ToolCallID.String))
		}
	}
	return messages
}
