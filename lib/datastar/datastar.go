// Package datastar wraps github.com/starfederation/datastar-go with the few
// helpers this app uses, so handlers get one import and the escaping rules
// for streamed text live in one place. The upstream types are re-exported
// rather than hidden, since the SSE generator is the main API surface.
package datastar

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	ds "github.com/starfederation/datastar-go/datastar"
)

// SSE is the upstream server-sent-event generator.
type SSE = ds.ServerSentEventGenerator

// Re-exported patch options, so callers only need this package.
var (
	WithSelectorID = ds.WithSelectorID
	WithModeAppend = ds.WithModeAppend
	WithModeInner  = ds.WithModeInner
)

// ReadSignals decodes Datastar signals from the request (query param for
// GET/DELETE, JSON body otherwise) into v.
func ReadSignals(r *http.Request, v any) error {
	return ds.ReadSignals(r, v)
}

// NewSSE sets the text/event-stream response headers and returns a generator;
// the first written event flushes them.
func NewSSE(w http.ResponseWriter, r *http.Request) *SSE {
	return ds.NewSSE(w, r)
}

// sseLineBreaks normalizes all line-break conventions to "\n" before a
// payload is split into data: lines. The upstream PatchElements splits on
// "\n"; a stray "\r" would otherwise survive into the payload.
var sseLineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// PatchTextAppend appends text to the element with the given id as an inert,
// HTML-escaped text node. Use it for untrusted content such as model output:
// patch elements are parsed as HTML, so unescaped text could inject markup.
// Empty text is a no-op (the upstream library would still emit an event).
func PatchTextAppend(sse *SSE, selectorID, text string) error {
	if text == "" {
		return nil
	}
	if err := sse.PatchElements(
		html.EscapeString(sseLineBreaks.Replace(text)),
		WithSelectorID(selectorID),
		WithModeAppend(),
	); err != nil {
		return fmt.Errorf("append text to #%s: %w", selectorID, err)
	}
	return nil
}
