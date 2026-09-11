package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent"
	ds "github.com/Napam/chatwego/lib/datastar"
	"github.com/Napam/chatwego/web/root"
	"github.com/google/uuid"
)

const maxTitleLen = 50

// sendPromptSignals is the subset of page signals the composer sends.
type sendPromptSignals struct {
	SessionID string `json:"sessionId"`
	Draft     string `json:"draft"`
}

func (h *Handlers) SendPrompt(w http.ResponseWriter, r *http.Request) {
	// ReadSignals must run before NewSSE: the SSE generator flushes the
	// response, and the request body is only readable until then.
	var signals sendPromptSignals
	if err := ds.ReadSignals(r, &signals); err != nil {
		h.logger.Error("Failed to read signals", slog.Any("error", err))
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	message := strings.TrimSpace(signals.Draft)
	if message == "" {
		http.Error(w, "Message required", http.StatusBadRequest)
		return
	}

	sessionID, isNewSession, err := h.resolveSession(
		r.Context(),
		signals.SessionID,
		message,
	)
	if err != nil {
		h.logger.Error("Failed to resolve session", slog.Any("error", err))
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	h.storeMessage(r.Context(), sessionID, "user", message)

	sse := ds.NewSSE(w, r)

	if isNewSession {
		if err := h.writeNewSessionPartials(r.Context(), sse, sessionID); err != nil {
			h.logger.Error("Failed to write new session partials", slog.Any("error", err))
			return
		}
	}

	// Clear the composer and hide the welcome placeholder.
	if err := sse.MarshalAndPatchSignals(map[string]any{
		"draft":       "",
		"hasMessages": true,
	}); err != nil {
		h.logger.Error("Failed to patch composer signals", slog.Any("error", err))
		return
	}

	if err := sse.PatchElementTempl(
		root.ChatContainer(root.MessageBubble{Message: message, ChatType: root.ChatTypeQuestion}),
		ds.WithModeAppend(),
		ds.WithSelectorID(root.ChatMessagesID),
	); err != nil {
		h.logger.Error("Failed to write user bubble partial", slog.Any("error", err))
		return
	}

	rounds, err := h.streamAssistantReply(r.Context(), sse, sessionID)
	if err != nil {
		if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
			h.logger.Debug("Stream aborted by client", slog.String("session_id", sessionID))
		} else {
			h.logger.Error("Failed to stream agent reply", slog.Any("error", err))
		}
	}

	h.storeRounds(context.WithoutCancel(r.Context()), sessionID, rounds)
}

// resolveSession returns the session to use for this prompt. It creates a new
// one when sessionID is empty or unknown.
func (h *Handlers) resolveSession(
	ctx context.Context,
	sessionID, message string,
) (id string, isNew bool, err error) {
	if sessionID != "" {
		if _, err := h.queries.GetSessionById(ctx, sessionID); err == nil {
			return sessionID, false, nil
		}
	}

	id = uuid.New().String()
	if _, err := h.queries.CreateChatSession(ctx, sqlc.CreateChatSessionParams{
		ID:    id,
		Title: sql.NullString{String: truncateTitle(message), Valid: true},
	}); err != nil {
		return "", true, fmt.Errorf("create chat session: %w", err)
	}
	return id, true, nil
}

func (h *Handlers) storeMessage(ctx context.Context, sessionID, msgType, content string) {
	if _, err := h.queries.CreateChatMessage(ctx, sqlc.CreateChatMessageParams{
		SessionID: sessionID,
		Type:      msgType,
		Content:   sql.NullString{String: content, Valid: true},
	}); err != nil {
		h.logger.Error(
			"Failed to store message",
			slog.String("type", msgType),
			slog.Any("error", err),
		)
	}
}

// storeRounds persists every round of an assistant reply: the assistant
// message itself (content, reasoning, and -- if it called tools -- the tool
// calls as a JSON blob), followed by one "tool" row per call result, so
// future replies in this session can be reconstructed with full tool-call
// context (see agent.buildMessages).
func (h *Handlers) storeRounds(ctx context.Context, sessionID string, rounds []agent.Round) {
	for _, round := range rounds {
		var toolCallsJSON sql.NullString
		if len(round.ToolCalls) > 0 {
			stored := make([]agent.StoredToolCall, len(round.ToolCalls))
			for i, toolCall := range round.ToolCalls {
				stored[i] = toolCall.StoredToolCall
			}
			data, err := json.Marshal(stored)
			if err != nil {
				h.logger.Error("Failed to marshal tool calls", slog.Any("error", err))
			} else {
				toolCallsJSON = nullableString(string(data))
			}
		}

		if _, err := h.queries.CreateChatMessage(ctx, sqlc.CreateChatMessageParams{
			SessionID: sessionID,
			Type:      "assistant",
			Content:   nullableString(round.Content),
			Reasoning: nullableString(round.Reasoning),
			ToolCalls: toolCallsJSON,
		}); err != nil {
			h.logger.Error("Failed to store assistant message", slog.Any("error", err))
		}

		for _, toolCall := range round.ToolCalls {
			if _, err := h.queries.CreateChatMessage(ctx, sqlc.CreateChatMessageParams{
				SessionID:  sessionID,
				Type:       "tool",
				Content:    nullableString(toolCall.Result),
				ToolCallID: sql.NullString{String: toolCall.ID, Valid: true},
			}); err != nil {
				h.logger.Error("Failed to store tool message", slog.Any("error", err))
			}
		}
	}
}

func nullableString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// writeNewSessionPartials pushes the sidebar, the sessionId signal, and a
// URL replace that only apply the first time a session is created.
func (h *Handlers) writeNewSessionPartials(
	ctx context.Context,
	sse *ds.SSE,
	sessionID string,
) error {
	chatSessions, err := h.queries.ListChatSessions(ctx)
	if err != nil {
		h.logger.Error("Failed to get sessions", slog.Any("error", err))
	}
	if err := sse.PatchElementTempl(
		root.SessionList(sessionID, chatSessions),
		ds.WithModeInner(),
		ds.WithSelectorID(root.SessionListID),
	); err != nil {
		return fmt.Errorf("write sidebar partial: %w", err)
	}
	if err := sse.MarshalAndPatchSignals(map[string]any{"sessionId": sessionID}); err != nil {
		return fmt.Errorf("write session-id signal: %w", err)
	}
	if err := sse.ReplaceURL(url.URL{Path: "/chats/" + sessionID}); err != nil {
		return fmt.Errorf("replace url: %w", err)
	}
	return nil
}

func (h *Handlers) streamAssistantReply(
	ctx context.Context,
	sse *ds.SSE,
	sessionID string,
) ([]agent.Round, error) {
	history, err := h.queries.GetMessagesBySession(ctx, sessionID)
	if err != nil {
		// Answering without history would drop the just-sent prompt from the
		// model's context, so fail instead of replying to nothing.
		return nil, fmt.Errorf("get message history: %w", err)
	}

	// Each round is persisted as its own row (see storeRounds), so it also
	// gets its own bubble here -- keeping live streaming and reload
	// consistent instead of merging every round into one bubble live but
	// splitting them on reload.
	var botBubbleID, reasoningID, toolCallsID string

	return h.agent.StreamReply(ctx, history, agent.ReplyHandlers{
		OnRoundStart: func() error {
			botBubbleID = "bot-" + uuid.New().String()
			reasoningID = "think-" + uuid.New().String()
			toolCallsID = "tools-" + uuid.New().String()
			return sse.PatchElementTempl(
				root.ChatContainer(root.MessageBubble{
					ChatType:    root.ChatTypeAnswer,
					ID:          botBubbleID,
					ReasoningID: reasoningID,
					ToolCallsID: toolCallsID,
				}),
				ds.WithModeAppend(),
				ds.WithSelectorID(root.ChatMessagesID),
			)
		},
		OnDelta: func(delta string) error {
			return ds.PatchTextAppend(sse, botBubbleID, delta)
		},
		OnReasoningDelta: func(delta string) error {
			return ds.PatchTextAppend(sse, reasoningID, delta)
		},
		OnToolCalls: func(calls []agent.ToolCallRecord) error {
			return sse.PatchElementTempl(
				root.ToolCallEntries(calls),
				ds.WithModeAppend(),
				ds.WithSelectorID(toolCallsID),
			)
		},
	})
}

func truncateTitle(s string) string {
	count := utf8.RuneCountInString(s)
	if count <= maxTitleLen {
		return s
	}
	r := []rune(s)
	return string(r[:maxTitleLen]) + "..."
}
