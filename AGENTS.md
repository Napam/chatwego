- Use the `tmp/` directory for temporary files when testing or experimenting
  (scratch scripts, test output, etc). It is gitignored.

- Keep yourself updated with what is in Taskfile.yml in order to ensure that you
  run correct commands.

- After doing changes, run static and build checks:
  - go code, run `task check.go`
  - web code, run `task check.web`
  - if both, run `task check` to check everything
  - to auto-fix formatting/linting, run `task fix` (or `task fix.go` / `task fix.web`)

- In `web/` avoid having inline svgs, always make icons at `web/lib/web-components/icons`

- Datastar drives the interactive UI. The client is vendored at
  `web/assets/vendor/datastar.js` (bundled to `static/assets/vendor/datastar.js`)
  and loaded per page as `<script type="module">`. Server endpoints stream with
  `lib/datastar` (a thin wrapper over datastar-go): decode page state with
  `datastar.ReadSignals`, open a stream with `datastar.NewSSE`, then patch
  elements or signals. Use `datastar.PatchTextAppend` for untrusted streamed
  text: it HTML-escapes the text so it lands as an inert text node.

- JS bundling (see `web/build.ts`):
  - `web/lib/**/*.ts` → single shared `static/bundle.js`, loaded synchronously
    in `<head>` (registers all web components before body parses).
  - `.ts` files in any other top-level dir (e.g. `web/pages/root.ts`, next to
    its `.templ`) → per-file page bundles in `static/page-files/<dir>/`,
    loaded per page with `<script defer>`.
  - Page files: avoid importing from `lib/`. Small utility imports are fine,
    but each duplicates code into the page bundle (iife has no code
    splitting). Don't import web components from `lib/`: re-registering them
    throws.

## Project shape

- HTTP entrypoint and routes: `cmd/serve/main.go`. Handlers live in
  `cmd/serve/handlers/`.
- Migrations are goose files in `db/migrations/`, queries are sqlc files in
  `db/queries/`, and generated code is in `db/generated/sqlc/`. Never hand-edit
  generated code; run `task gen.db`.
- The agent lives in `lib/agent`: the streaming loop in `agent.go`, tools in
  `lib/agent/tools/` (one file per tool, self-registered into `tools.All`),
  skills as markdown files in `lib/agent/skills/`.
- `cmd/mockllm` is a deterministic OpenAI-compatible server for offline UI
  testing. Run it with `task dev.mock-llm`; it serves on the default
  `OPENAI_BASE_URL` (`localhost:6700`).
- Read `docs/architecture.md` before changing the streaming path or the agent
  loop.

## Conventions

- Streaming endpoints use `lib/datastar`, not `datastar-go` directly. Decode
  signals with `datastar.ReadSignals` before opening the stream with
  `datastar.NewSSE`.
- Stream untrusted model text with `datastar.PatchTextAppend`. Element patches
  are parsed as HTML, so raw text has to stay escaped.
- Use `htmlclass.Join` for plain class joins and `twmerge.Merge` only when
  utilities actually conflict (component overrides, active-state toggles).
- Message history is ordered by `id`, not `created_at`. Keep live streaming and
  reload rendering consistent: one bubble per persisted round.
- Reference assets, bundles, and icons through `chatwego.StaticRootPath`; don't
  hardcode `/static/...`.
