# ChatWeGo: general chatbot on Datastar

Goal: turn this scaffold into a general chatbot app with the same feature set as
`../chatbase`, but using Datastar instead of HTMX. Keep the chatbase architecture
and UX where it already works, swap the transport and the templ attributes.

References used:

- `../backwego-template`: the scaffold this repo came from.
- `../chatbase`: full-parity source (sessions, streaming, markdown, agent tools
  and skills, admin).
- `../apato`: working example of templ + Datastar + Lit web components, plus the
  `datastar-go` server usage (`web/endpoints/messages`, `lib/utils/datastarutils`).
- Datastar reference: https://data-star.dev/reference
- Vendored client: `web/assets/vendor/datastar.js` (v1.0.3).

Decisions from the user:

- Full parity with chatbase.
- Port the cookie/session identity model.
- Best-effort OpenAI-compatible LLM (one `OPENAI_BASE_URL` + `OPENAI_MODEL`
  covers llama.cpp, Ollama, vLLM, LM Studio, and OpenAI).

---

## 1. Findings

### 1.1 Build and asset pipeline

- `web/build.ts` bundles two things:
  - `web/lib/**/*.ts` into one `static/bundle.js`, loaded synchronously in
    `<head>` so all custom elements register before body parse.
  - every other top-level `web/**/*.ts` into `static/page-files/<dir>/<name>.js`,
    loaded per page with `defer`.
- `web/assets/**/*` is hardlinked into `static/assets/**`, mirroring subdirs.
  This already covers the vendored client: `web/assets/vendor/datastar.js`
  becomes `static/assets/vendor/datastar.js`. Reference it with
  `chatwego.StaticRootPath("static/assets/vendor/datastar.js")`.
- In dev, `files_noembed.go` serves `web/static` from disk with `no-store` and
  `StaticRootPath` returns the plain path. In prod, `files_embed.go` embeds
  `web/static/*` and `hashfs` content-hashes URLs. Both already handle nested
  directories via the `*` pattern.
- Tailwind v4 is configured in `web/tailwind.css` via `@theme`. The chatbase
  additions we need: `@plugin '@tailwindcss/typography'`, `--color-neutral-750`,
  `--color-neutral-850`, and the `--animate-fade-in` keyframes.

### 1.2 Datastar client v1.0.3 (vendored)

The file is an ES module and must load as `<script type="module" src=...>`.
It auto-initializes: the first attribute registration schedules a `setTimeout`
that walks `document.documentElement` and installs a MutationObserver, so DOM
added later is processed too.

Attributes used here:

- `data-signals` / `data-signals:<key>`: patch signals. Keys are camel-cased.
  Signal names starting with `_` are excluded from requests by default.
- `data-bind`: two-way binds an input to a signal (default `input` event for
  textareas/inputs, `value` + `change` for generic elements).
- `data-on:<event>`: event listener. `data-on:submit` auto-calls
  `preventDefault` on a form. Supports `__prevent`, `__debounce`, etc.
- `data-text`, `data-show`, `data-class`, `data-style`, `data-attr`, `data-ref`,
  `data-indicator`, `data-init`, `data-effect`, `data-computed`.

Actions:

- `@get/@post/@put/@patch/@delete(uri, options)`. Default `contentType` is
  `json`: all signals (except `_`-prefixed) are sent as a JSON body, and
  `Datastar-Request: true` is set.
- Options include `requestCancellation` (`'auto'`, `'cleanup'`, `'disabled'`, or
  an `AbortController`), `filterSignals`, `openWhenHidden`, `retry`.

Response handling:

- `text/html` -> patch elements, with `datastar-selector` / `datastar-mode` /
  `datastar-use-view-transition` response headers.
- `application/json` -> patch signals.
- `text/javascript` -> execute script.
- `text/event-stream` -> Datastar SSE events.

SSE wire format (what `datastar-go` emits):

```
event: datastar-patch-elements
data: selector #foo
data: mode append
data: elements <div>...</div>
<blank line>

event: datastar-patch-signals
data: signals {"draft":""}
<blank line>
```

Modes: `outer` (default, morphs by id), `inner`, `replace`, `prepend`, `append`,
`before`, `after`, `remove`. Repeated `data:` lines for the same key are joined
with `\n`, so multi-line element payloads survive.

Lifecycle events: `datastar-fetch` fires on `document` with
`detail.type` in `started`, `finished`, `error`, `retrying`, `retries-failed`.
This replaces chatbase's `htmx:sse:*` hooks.

### 1.3 `datastar-go` v1.2.2 (server)

Added as a Go dependency. Relevant API (framework-agnostic; takes
`http.ResponseWriter` + `*http.Request`):

- `datastar.ReadSignals(r, &signals)`: GET/DELETE reads the `datastar` query
  param, other methods read the JSON body.
- `datastar.NewSSE(w, r)`: sets `text/event-stream`, flushes headers, returns a
  generator. Keep the handler open to stream.
- `sse.PatchElementTempl(component, opts...)`, `sse.PatchElements(html, opts...)`.
  Options: `WithSelectorID(id)`, `WithSelector(sel)`, `WithModeAppend()`,
  `WithModeInner()`, `WithModeRemove()`, etc.
- `sse.PatchSignals(jsonBytes)`, `sse.MarshalAndPatchSignals(v)`.
- `sse.ExecuteScript(js)`, `sse.ReplaceURL(url)`, `sse.Redirect(url)`.
- `sse.Context()`, `sse.IsClosed()`, `sse.Send(...)`.

`PatchElementTempl` only requires a structural `Render(ctx, io.Writer) error`
interface, so it works with templ components and adds no templ dependency.

### 1.4 chatbase backend

- `cmd/serve/main.go`: `run(logger)` wires DB, migrations, agent, `SetupRouter`.
- `cmd/serve/handlers/`:
  - `index.go`: list owner's sessions, render root page.
  - `chats.go`: render one session's messages, with ownership check.
  - `send_prompt.go`: resolve/create session, store user message, open SSE,
    stream assistant rounds, persist rounds.
  - `sse.go`: htmx partial writers (to be replaced with Datastar).
  - `admin.go`: render static tools/skills page.
- `lib/agent`: OpenAI-compatible streaming agent, tool-call loop (max 25
  rounds), reasoning content passthrough, history reconstruction.
  - `lib/agent/tools`: self-registering tool registry (`tools.All`).
  - `lib/agent/skills`: markdown skills with YAML frontmatter, embedded.
- `lib/session`: cookie session middleware, hashed tokens in `session_tokens`,
  falls back to bootstrapping a system-identity session.
- `lib/identity`: system identity/login constants.
- `lib/htmlclass.Join`: readability helper for class attributes.

Migrations:

- `000001_init_auth.sql`: `identities`, `logins`, `session_tokens`,
  `identity_roles`, plus a seeded system identity and system login.
- `000002_init_chat.sql`: `chat_sessions` (with `identity_id`), `chat_messages`,
  and a trigger bumping `chat_sessions.updated_at`.
- sqlc queries in `db/queries/auth.sql` and `db/queries/chatsessions.sql`.

### 1.5 chatbase frontend

- `root.templ`: session sidebar, chat message list, welcome placeholder,
  reasoning disclosure, tool-call disclosure, message bubble, composer.
- `root.ts`: page script with four subsystems: thoughts sync (loading label,
  user pin, finalize), follow-mode scroll, frame batching, stream lifecycle.
- `markdown-message.ts` + `markdown-reconcile.ts`: Lit component that renders
  markdown (markdown-it + highlight.js + task lists) and diffs streamed output
  into the DOM with a per-chunk fade. Static mode reads light-DOM text; streaming
  mode observes a sibling accumulator element by id.

The markdown pipeline is plain browser code and does not depend on HTMX, so it
ports as-is.

### 1.6 apato conventions

- `datastarutils.DSHandlerFunc[T]` decodes signals, then calls the handler.
- `NewSSE(c)` wraps `datastar.NewSSE`.
- SSE patch helpers use `WithSelectorID` + `WithModeInner`/`WithModeAppend`.
- Datastar loads last as a `type="module"` script; `bundle.js` stays
  synchronous in `<head>`.
- Form pattern: `data-on:submit__prevent` + `data-bind` + `@post`.
- Enter-to-send: page script calls `form.requestSubmit()`.

---

## 2. Key design decisions

### 2.1 Streaming transport

Each prompt is one `POST /send-prompt` that returns an SSE stream and ends when
the reply finishes. This mirrors chatbase (no persistent stream), which keeps
the session model simple.

Server patch sequence:

1. New session only: patch `#session-list` inner, patch the `sessionId` signal,
   and run a small script to `history.replaceState` the URL.
2. Append the user bubble to `#chat-messages`.
3. Remove/hide the welcome placeholder by patching `{"hasMessages": true}` and
   using `data-show="!$hasMessages"`.
4. Clear `draft` via signal patch.
5. Per assistant round: append a bubble shell with a hidden accumulator span
   (`#bot-...`), a reasoning accumulator (`#think-...`), and a tool-calls
   container (`#tools-...`).
6. Per reply/reasoning delta: `PatchElements(html.EscapeString(delta),
   selector=#bot-..., mode=append)`. Escaped text parses back to a text node, so
   the accumulator stays clean raw markdown for `markdown-message`.
7. Tools: append rendered `ToolCallEntries` into `#tools-...`.
8. On finish: page script finalizes the UI from the `datastar-fetch` event.

Why append escaped text instead of a signal: it is O(delta) per token, does not
resend the whole answer, and feeds the existing MutationObserver-based markdown
component unchanged.

### 2.2 Composer and stop button

The form holds `data-signals` for `draft`, `sessionId`, `hasMessages`, plus an
underscore-prefixed `_controller` (excluded from requests) holding an
`AbortController`.

Submit expression:

```
$_controller.abort();
$_controller = new AbortController();
@post('/send-prompt', {requestCancellation: $_controller});
$draft = '';
```

`@post` captures the payload synchronously before the expression continues, so
clearing `draft` after the call is safe. Fresh controller per submit means a
second submit cancels the first, and the stop button is just
`data-on:click="$_controller.abort()"`.

Enter-to-send is a `data-on:keydown` expression on the textarea calling
`el.form.requestSubmit()` unless Shift or IME composition.

### 2.3 Keep the Lit markdown components

Port `markdown-message.ts` and `markdown-reconcile.ts` unchanged into
`web/lib/web-components/`. Add `highlight.js`, `markdown-it`,
`markdown-it-task-lists`, and `@tailwindcss/typography`.

### 2.4 Identity and sessions

Port chatbase's auth tables, sqlc queries, and `lib/session` middleware. Every
visitor gets a system-identity session cookie, and chat sessions are scoped to
the resolved identity. Real login can be added later without schema changes.

### 2.5 Class helpers

Keep both, because they solve different problems:

- `lib/htmlclass.Join` (`htmlclass.Join`): plain space-join for class strings
  built from several pieces. No parsing, no dedupe, cheap. Default choice when
  there are no conflicting utilities.
- `github.com/Oudwins/tailwind-merge-go` (`twmerge.Merge`): parse and merge so
  the last conflicting utility wins. Use for overriding component defaults and
  for conditional toggles, e.g. the active session row in the sidebar.

### 2.6 Agent

Port the OpenAI-compatible agent, its tool registry, and its skills verbatim
(import paths rewritten). Config via `OPENAI_BASE_URL` (default
`http://localhost:6700`, the llama.cpp/llama-server default) and `OPENAI_MODEL`
(default `gpt-4o-mini`). Also add `DB_PATH`.

---

## 3. Database reset

The scaffold's `000001_init.sql` creates a demo `users` table, and the local
`data/sqlite.db` already has migration 1 applied. Replacing it with the auth
migration means goose would skip the new migration 1. Local DBs only ever hold
test data in this repo, so deleting them is routine and needs no migration
dance. Concretely:

- delete `data/sqlite.db` (gitignored) so migrations run from scratch;
- delete `db/migrations/000001_init.sql`, `db/queries/users.sql`, and
  `db/generated/sqlc/users.sql.go`.

## 4. Work breakdown

### Stage A: data layer

- Replace migrations: add `db/migrations/000001_init_auth.sql`,
  `db/migrations/000002_init_chat.sql`; remove the demo migration.
- Replace queries: add `db/queries/auth.sql`, `db/queries/chatsessions.sql`;
  remove `db/queries/users.sql`.
- Remove generated `users.sql.go`; run `task gen.db`.

### Stage B: Go libraries

- `lib/env`: add `OpenAIModel`, `OpenAIBaseURL`, `DBPath`.
- `lib/identity/identity.go`: system identity constants.
- `lib/session/session.go`: cookie session middleware and context helpers.
- `lib/htmlclass/htmlclass.go`: `Join` (plain space-join).
- Add `github.com/Oudwins/tailwind-merge-go` and use `twmerge.Merge` where
  utilities conflict (active session row, component overrides).
- `lib/agent/**`: agent, tools, skills, plus the embedded skill markdown.
- `lib/datastar/datastar.go` (new): small chi-friendly helpers:
  `ReadSignals`, `NewSSE`, `PatchElement`, `PatchText`, `PatchSignals`,
  `RemoveElement`, mirroring apato's `datastarutils`.

### Stage C: HTTP layer

- Rewrite `cmd/serve/main.go` to a `run(logger)` + `SetupRouter` shape with
  request logging, `Recoverer`, session middleware, static setup, and routes.
- Add `cmd/serve/handlers/`: `handlers.go`, `index.go`, `chats.go`,
  `send_prompt.go`, `admin.go`. No htmx SSE helpers.

### Stage D: web

- `web/package.json`: add markdown/highlight/typography deps.
- `web/tailwind.css`: add typography plugin, neutral-750/850, fade-in.
- Port `markdown-message.ts`, `markdown-reconcile.ts`.
- Rewrite `web/root/root.templ` for Datastar.
- Rewrite `web/root/root.ts` lifecycle for `datastar-fetch`.
- Add `web/admin/admin.templ`.
- Load `datastar.js` as a module in both pages.

### Stage E: docs and verification

- Update `README.md` and `AGENTS.md` (Datastar instead of htmx as the intended
  framework, new env vars, database flow).
- `bun install`, `task gen.templ`, `task gen.db`, `task check`.
- Manual smoke test with `task dev`.

## 5. Risks and things to watch

- `ReadSignals` must run before `NewSSE` for POST bodies (see the library's
  error message about a closed body).
- Datastar morphs by id. Every live element uses a stable unique id; we only
  append for bubbles, and morph `inner` for the sidebar.
- `PatchSignals` payloads must be valid `data-signals` syntax (JSON is fine).
- Aborting with a custom `AbortController` disables Datastar's auto-cancel, so
  the submit expression must abort the previous controller explicitly.
- Removing `#welcome-placeholder` from the server logs a "no targets found"
  warning, so it is hidden with `data-show` instead.
- The vendored client and `datastar-go` are protocol-compatible; the Go library
  emits the same `datastar-patch-*` events.
- go embed of `web/static/*` already includes nested asset and page-file dirs.

## 6. Verification

- `task gen.db` and `task gen.templ` produce no drift.
- `task check` passes (Go build/lint, prettier/eslint/tsc, bundle build).
- Smoke test with `task dev`:
  - first visit sets a session cookie and shows an empty chat;
  - sending a prompt creates a session, streams a markdown reply, and updates
    the sidebar;
  - reload renders stored messages identically;
  - reasoning and tool-call disclosures render when the model emits them;
  - stop aborts mid-stream; a second submit cancels an in-flight one;
  - `/admin` lists tools and skills.
