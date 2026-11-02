package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func testClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func TestStreamsReasoningContentAndDone(t *testing.T) {
	srv := httptest.NewServer(handler(time.Millisecond))
	defer srv.Close()

	resp, err := testClient().Post(
		srv.URL+"/v1/chat/completions",
		"application/json",
		strings.NewReader(`{"stream":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var content, reasoning strings.Builder
	var done, finished, sawRole bool
	for _, line := range strings.Split(string(body), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			done = true
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("invalid chunk %q: %v", data, err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Role == "assistant" {
				sawRole = true
			}
			content.WriteString(choice.Delta.Content)
			reasoning.WriteString(choice.Delta.ReasoningContent)
			if choice.FinishReason == "stop" {
				finished = true
			}
		}
	}

	if !done {
		t.Error("stream has no [DONE]")
	}
	if !finished {
		t.Error("stream has no finish_reason stop")
	}
	if !sawRole {
		t.Error("stream has no initial assistant role delta")
	}
	if !strings.Contains(reasoning.String(), "Checking the request") {
		t.Errorf("reasoning = %q, want the canned trace", reasoning.String())
	}
	if !strings.Contains(content.String(), "Hello from the mock model!") {
		t.Errorf("content = %q, want the canned reply", content.String())
	}
	// Newlines must survive streaming, otherwise markdown lists and fences
	// collapse into a single paragraph.
	if !strings.Contains(content.String(), "\n\n- streaming works") {
		t.Errorf("content = %q, want preserved newlines", content.String())
	}
}

func TestStreamsToolCallForMocktoolPrompt(t *testing.T) {
	srv := httptest.NewServer(handler(time.Millisecond))
	defer srv.Close()

	body := `{"stream":true,"messages":[{"role":"user","content":"use MOCKTOOL"}]}`
	resp, err := testClient().Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`"tool_calls"`,
		`"get_current_time"`,
		`"finish_reason":"tool_calls"`,
		"data: [DONE]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tool response does not contain %q", want)
		}
	}
}

func TestFollowUpAfterToolResultStreamsText(t *testing.T) {
	srv := httptest.NewServer(handler(time.Millisecond))
	defer srv.Close()

	body := `{"stream":true,"messages":[` +
		`{"role":"user","content":"use MOCKTOOL"},` +
		`{"role":"tool","content":"2026-01-01T00:00:00Z"}]}`
	resp, err := testClient().Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, `"tool_calls"`) {
		t.Errorf("follow-up response still calls tools: %s", got)
	}
	if !strings.Contains(got, `"content":"Hello `) {
		t.Errorf("follow-up response has no text content: %s", got)
	}
}

func TestNonStreamingCompletion(t *testing.T) {
	srv := httptest.NewServer(handler(time.Millisecond))
	defer srv.Close()

	resp, err := testClient().Post(
		srv.URL+"/v1/chat/completions",
		"application/json",
		strings.NewReader(`{"stream":false}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q, want application/json", ct)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		t.Fatal(err)
	}
	if len(completion.Choices) != 1 {
		t.Fatalf("choices = %d, want 1", len(completion.Choices))
	}
	if !strings.Contains(completion.Choices[0].Message.Content, "Hello from the mock model!") {
		t.Errorf("content = %q, want the canned reply", completion.Choices[0].Message.Content)
	}
}

func TestRejectsUnknownPaths(t *testing.T) {
	srv := httptest.NewServer(handler(time.Millisecond))
	defer srv.Close()

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/v1/embeddings"},
		{http.MethodGet, "/v1/embeddings"},
	} {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := testClient().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want 404", tc.method, tc.path, resp.StatusCode)
		}
	}
}
