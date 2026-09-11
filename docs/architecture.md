# Architecture

How ChatWeGo fits together: the HTTP routes, the streaming protocol, the agent
loop, and the data model. The [README](../README.md) covers setup and the
build. Source comments carry the fine detail; this is the map.

## Overview

One Go binary serves both the app and its web assets. templ renders the HTML,
Datastar streams updates over SSE, and SQLite stores chat sessions and
messages. The frontend is a small page script plus Lit web components for
markdown rendering. There is no client-side framework and no separate API.

```
browser ──templ HTML──> cmd/serve/handlers
   │                         │
   │  POST /send-prompt      │
   │  (Datastar signals)     ├─> lib/agent ──> OpenAI-compatible API
   │ <──SSE patches──────────┤
   │                         └─> db/generated/sqlc ──> SQLite
   │
   └─ pages/root.ts + Lit components render what the patches append
```

## Request lifecycle

Routes are registered in `cmd/serve/main.go`:

| Method | Path | Handler | Purpose |
| --- | --- | --- | --- |
| GET | `/` | `Index` | New chat: sidebar plus the welcome placeholder. |
| GET | `/chats/{sessionID}` | `GetChat` | Existing chat. |
| POST | `/send-prompt` | `SendPrompt` | Store the prompt, then stream the reply over SSE. |
| GET | `/admin` | `Admin` | Read-only list of loaded tools and skills. |
| GET | `/static/*` | static | Hashed assets in prod, disk with `no-store` in dev. |

Middleware, in order: request logging, then `chi`'s `Recoverer`. There is no
auth middleware; all sessions and messages are global. Logs are `slog` JSON on
stdout; `task dev` renders them with `hl`.

### Sending a prompt

`SendPrompt` is the core path:

1. Decode signals (`sessionId`, `draft`) with `datastar.ReadSignals`. This must
   happen before the SSE stream opens, because the response flush ends the
   window in which the request body is readable.
2. Resolve the session. An empty or unknown id creates a new one.
3. Persist the user message.
4. Open the SSE stream.
5. First prompt in a session only: morph the sidebar by id to include the new
   session, patch the `sessionId` signal, and `ReplaceURL` to `/chats/<id>`.
6. Patch signals to clear `draft` and set `hasMessages`, hide the welcome
   placeholder, and append the user bubble.
7. Stream the assistant reply (see [Agent](#agent)), then persist every round.

Persisting after streaming uses `context.WithoutCancel`, so a client that
disconnects mid-reply still gets the partial reply saved.

## Streaming and the client

### Wire format

Datastar carries two event types; `lib/datastar` wraps `datastar-go`:

```
event: datastar-patch-elements
data: selector #chat-messages
data: mode append
data: elements <div>...</div>

event: datastar-patch-signals
data: signals {"draft":""}
```

Element patches morph by id by default; the app uses `append` for new bubbles
and accumulator text, and morphs the whole sidebar in place by id.
`PatchTextAppend` appends model output as an HTML-escaped inert text node.
Patches are parsed as HTML, so
unescaped model text could inject markup; `PatchTextAppend` is the only path for
untrusted streamed text and it never touches the raw HTML.

### Composer and stop

The form and body hold the page's signals: `draft`, `sessionId`,
`hasMessages`, plus `_controller`, an `AbortController`. Signals prefixed with
`_` are excluded from requests by Datastar's default signal filter.

On submit the expression aborts any previous controller, replaces it, and
`POST`s to `/send-prompt` with `requestCancellation: $_controller` and
`retry: 'never'`. A fresh controller per submit means a second prompt cancels
the first. The stop button just aborts that controller. `@post` captures the
payload synchronously, so clearing `draft` afterward is safe.

### Page script

`web/pages/root.ts` has four subsystems:

- **Thoughts sync:** toggles each round's reasoning disclosure between the
  pulsing "Thinking ..." label and "Thoughts", tracks whether the user opened it
  by hand, and hides empty disclosures at stream end.
- **Follow mode:** pins the message list to the bottom while streaming, and
  stops when the user scrolls up. Each reasoning body follows independently.
- **Frame batching:** a `MutationObserver` coalesces DOM changes into one
  `requestAnimationFrame` pass that syncs thoughts and scrolls. Datastar patches
  are DOM mutations, so the observer sees every delta.
- **Stream lifecycle:** listens for the `datastar-fetch` event and reacts only
  when `detail.el` is the composer form. `started` shows the stop button and
  sets the liveness flag; `finished` or `error` settles the markdown, finalizes
  disclosures, and hides the stop button. Aborting counts as finished.

### Markdown components

`markdown-message.ts` renders markdown with markdown-it, highlight.js, and
task lists, then diffs each parsed chunk into the DOM through
`markdown-reconcile.ts` with a per-chunk fade. It reads its input two ways:
static mode takes light-DOM text, and streaming mode observes a sibling
accumulator span by id. When the reply settles, the page script sets a
`settled` attribute so the final render matches a reload.

## Agent

`lib/agent` wraps the OpenAI-compatible chat completions API and streams the
reply through callbacks (`ReplyHandlers`). Sessions are reconstructed from the
message history on every request; there is no in-memory conversation state.

### Rounds

One reply is one or more rounds. A round is one request/response with the model.
If the model returns tool calls, the agent runs them, appends the assistant
tool-call message and one tool-result message per call, and loops. When the
model returns text with no tool calls, the reply ends.

Each round is a separate round trip and a separate persisted row. Live streaming
gives every round its own bubble, so what streams in matches what a reload
renders.

### Tool loop and cap

Tool calls run locally and synchronously; the results feed the next round.
After `maxToolRounds` (25), the request drops the `Tools` list and appends a
system notice telling the model to answer directly. Both are needed: dropping
the tools guarantees termination at the API level, and the notice stops models
like llama.cpp that emit tool-call syntax as learned text tokens from doing so
anyway. The notice is request-only and never persisted.

### Reasoning

Reasoning models on OpenAI-compatible servers (llama.cpp and others) stream a
`reasoning_content` field that is not part of the typed schema; the agent reads
it from the raw extra fields and forwards it through `OnReasoningDelta`.
Reasoning is stored on the assistant row and rendered as a collapsible
"Thoughts" disclosure.

### System prompt and skills

The system prompt is rendered once at package init. It includes a summary of
every loaded skill (name and description) so the model knows what it can fetch
with the `skill` tool. Skill bodies are loaded from embedded markdown, not sent
up front. Tools come from `tools.All`, another package-level registry.

Both registries follow the same pattern: add a file, not an entry in a list. A
tool file declares a package-level `var _ = register(tools.Tool{...})`; a skill
is a `.md` file with YAML frontmatter. See the README for examples.

## Data model

One migration, applied by goose and embedded in the binary:

- `000001_init_chat.sql`: `chat_sessions` and `chat_messages`, plus a trigger
  that bumps `chat_sessions.updated_at` on every insert.

`chat_messages` is a multi-role table: `user`, `assistant`, and `tool` rows all
live in one table so the whole history can be passed to the model in order. An
assistant row may carry `reasoning` and a `tool_calls` JSON array; each `tool`
row carries the result text and the `tool_call_id` linking it back. `system`
rows are supported by the schema but the current app sends its system prompt
itself.

History is ordered by `id ASC`, not `created_at`: `id` is a monotonic rowid,
while `created_at` has one-second resolution and can order same-second rows
nondeterministically.

One reply is persisted in a single transaction, the assistant row and its tool
rows together, so a partial write can't leave tool results without their
assistant message. A round with no content, reasoning, or tool calls (a stream
that failed or was aborted before any output) is not persisted.

## Invariants and gotchas

- `ReadSignals` must run before `NewSSE` on POST requests.
- Every live patch target uses a stable unique id; Datastar morphs by id.
- Streamed model text goes through `PatchTextAppend` only.
- A custom `AbortController` disables Datastar's auto-cancel, so the submit
  expression has to abort the previous controller itself.
- The welcome placeholder is hidden with `data-show`, not removed, because
  patching a missing selector logs a "no targets found" warning.
- There is no `WriteTimeout` on the HTTP server: an SSE reply stays open for the
  whole model response.
- SQLite runs with `foreign_keys`, `busy_timeout(5000)`, and WAL enabled through
  the DSN (`openSQLite` in `cmd/serve/main.go`); foreign keys are off by default
  in SQLite, so the schema's `ON DELETE CASCADE` only works because of this.
