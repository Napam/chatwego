// Command mockllm is a deterministic OpenAI-compatible chat completions
// server for local development and browser tests. It streams a fixed reply
// with reasoning deltas, so the chat UI can be exercised without a real model.
//
// Send "MOCKTOOL" in a prompt to make the first round return a get_current_time
// tool call. The follow-up round, which includes the tool result, streams the
// normal markdown reply. That exercises the agent's tool loop end to end.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	model         = "mock-model"
	reasoningText = "Checking the request, then writing a short reply."
	contentText   = "Hello from the mock model!\n\n" +
		"This reply tests **markdown**, a list, and a code block:\n\n" +
		"- streaming works\n" +
		"- no real model is involved\n\n" +
		"```go\nfmt.Println(\"hello\")\n```\n"
)

func main() {
	addr := flag.String("addr", "localhost:6700", "listen address")
	delay := flag.Duration("delay", 20*time.Millisecond, "delay between streamed words")
	flag.Parse()

	log.Printf("mock OpenAI server on http://%s", *addr)
	if err := http.ListenAndServe(*addr, handler(*delay)); err != nil {
		log.Fatal(err)
	}
}

// completionRequest is the subset of the request body the mock cares about.
type completionRequest struct {
	Stream   *bool `json:"stream"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// wantsToolCall reports whether the first round should answer with a tool
// call: the prompt asked for MOCKTOOL and no tool result is in the history yet.
func (r completionRequest) wantsToolCall() bool {
	wants, hasResult := false, false
	for _, m := range r.Messages {
		if m.Role == "tool" {
			hasResult = true
		}
		if bytes.Contains(m.Content, []byte("MOCKTOOL")) {
			wants = true
		}
	}
	return wants && !hasResult
}

// handler serves chat completions and a model list. Any POST path ending in
// /chat/completions is accepted so both OpenAI and llama.cpp style base URLs
// work without configuration.
func handler(delay time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   []map[string]string{{"id": model, "object": "model"}},
		})
	})
	// Unknown GET paths would otherwise get a 405 from the POST pattern.
	mux.HandleFunc("GET /", http.NotFound)
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var req completionRequest
		_ = json.Unmarshal(body, &req)
		if req.Stream != nil && !*req.Stream {
			writeCompletion(w)
			return
		}
		if req.wantsToolCall() {
			streamToolCall(w, r, delay)
			return
		}
		streamReply(w, r, delay)
	})
	return mux
}

// writeCompletion answers a non-streaming request with one completion object.
func writeCompletion(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]string{
				"role":              "assistant",
				"content":           contentText,
				"reasoning_content": reasoningText,
			},
			"finish_reason": "stop",
		}},
	})
}

// streamReply writes one SSE reply: reasoning first, then markdown content,
// then a stop chunk and [DONE]. Writing stops when the client disconnects.
func streamReply(w http.ResponseWriter, r *http.Request, delay time.Duration) {
	s, ok := newEventStream(w)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	if !s.send(map[string]any{"role": "assistant", "content": ""}, "") {
		return
	}
	for _, word := range strings.Fields(reasoningText) {
		if !sleepOrDone(r, delay) || !s.send(map[string]any{"reasoning_content": word + " "}, "") {
			return
		}
	}

	// SplitAfter keeps the newlines that separate paragraphs, lists, and code
	// fences, so the client renders real markdown rather than one long line.
	for _, word := range strings.SplitAfter(contentText, " ") {
		if !sleepOrDone(r, delay) || !s.send(map[string]any{"content": word}, "") {
			return
		}
	}

	if s.send(map[string]any{}, "stop") {
		s.done()
	}
}

// streamToolCall writes a single-round tool call to get_current_time, followed
// by a tool_calls finish. The agent runs the tool and sends another request.
func streamToolCall(w http.ResponseWriter, r *http.Request, delay time.Duration) {
	s, ok := newEventStream(w)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	first := map[string]any{
		"role": "assistant",
		"tool_calls": []map[string]any{{
			"index": 0,
			"id":    "call_mock_1",
			"type":  "function",
			"function": map[string]string{
				"name":      "get_current_time",
				"arguments": "",
			},
		}},
	}
	if !s.send(first, "") {
		return
	}
	args := map[string]any{
		"tool_calls": []map[string]any{{
			"index":    0,
			"function": map[string]string{"arguments": "{}"},
		}},
	}
	if !sleepOrDone(r, delay) || !s.send(args, "") {
		return
	}
	if s.send(map[string]any{}, "tool_calls") {
		s.done()
	}
}

// eventStream writes data-only SSE events to w.
type eventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newEventStream(w http.ResponseWriter) (*eventStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &eventStream{w: w, flusher: flusher}, true
}

func (s *eventStream) send(delta map[string]any, finish string) bool {
	payload := map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         delta,
			"finish_reason": finish,
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", data); err != nil {
		return false
	}
	s.flusher.Flush()
	return true
}

func (s *eventStream) done() {
	_, _ = io.WriteString(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

// sleepOrDone waits for delay and reports false if the client went away.
func sleepOrDone(r *http.Request, delay time.Duration) bool {
	select {
	case <-time.After(delay):
		return true
	case <-r.Context().Done():
		return false
	}
}
