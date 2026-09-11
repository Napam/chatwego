# ChatWeGo

A self-hosted chatbot. It streams markdown replies, shows reasoning traces and
tool calls, keeps a per-session sidebar, and talks to any OpenAI-compatible
chat completions API (llama.cpp, Ollama, vLLM, LM Studio, or OpenAI itself).

The server renders HTML with templ and streams updates with
[Datastar](https://data-star.dev/) over SSE. SQLite stores chat sessions and
messages; goose manages migrations and sqlc generates the queries.
Everything compiles into a single binary.

## Features

- Streaming markdown replies with syntax highlighting, tables, and task lists
- Collapsible reasoning and tool-call disclosures, both live and on reload
- Per-session sidebar with URL-backed sessions (`/chats/<id>`)
- Stop button; submitting a new prompt cancels the in-flight reply
- Tool registry and markdown skills, both listed on `/admin`
- Single-user and login-free; chat history is shared
- Web assets and migrations embedded in the production binary

## Configuration

| Variable              | Default                 | Purpose                                                                              |
| --------------------- | ----------------------- | ------------------------------------------------------------------------------------ |
| `OPENAI_BASE_URL`     | `http://localhost:6700` | Base URL of the OpenAI-compatible API (the llama.cpp/llama-server default).          |
| `OPENAI_MODEL`        | `gpt-4o-mini`           | Chat model to request.                                                               |
| `OPENAI_API_KEY`      | unset                   | Read by the OpenAI client and sent as a bearer token when the provider requires one. |
| `DB_PATH`             | `./data/sqlite.db`      | SQLite database file.                                                                |
| `HOST` / `PORT`       | `localhost` / `5080`    | Listen address.                                                                      |
| `DB_MIGRATE_ON_START` | `false`                 | Run goose migrations on startup. `task dev` sets it to `true`.                       |

Requirements:

- [mise](https://mise.jdx.dev) for development environment setup - mise must be
  [activated](https://mise.jdx.dev/getting-started.html#activate-mise).

## Quickstart

1. Run `task dev`. It installs the mise tools and JS deps, symlinks the git
   pre-push hook, and starts the dev server with live reload.
2. Open the live reloading proxy at `localhost:5081` (set via
   `LIVE_RELOAD_PROXY_PORT` in `Taskfile.yml`). The app itself listens on `PORT`
   (default `5080`).
3. Start making changes: the application entrypoint is `cmd/serve/main.go` and
   the frontend root page is `web/root/root.templ`.

For a production build, run `task build.go` (output in `bin/app`).

Docker:

```sh
task build.docker       # build image
task build.docker.run   # build and run
```

## How it works

The composer `POST`s to `/send-prompt` and Datastar streams the reply back over
SSE. Each assistant round appends a bubble with hidden accumulators for the
reply text and reasoning; every delta is appended as an inert text node, and the
`<markdown-message>` web component (markdown-it + highlight.js) re-renders the
accumulator live. Tool calls render as collapsible disclosures. When the first
prompt creates a session, the server patches both the sidebar and the session
URL.

The app is single-user and has no login. Every visitor sees the same session
sidebar and chat history.

- Production builds embed web assets and migrations into the binary; dev mode
  serves them from disk so changes show up on reload without a rebuild (see
  [Web assets](#web-assets)).
- Web components with shadow DOM and shared TailwindCSS (with working dark mode
  toggling).
- A Dockerfile that builds a minimal scratch image.
- golangci-lint, eslint, and prettier preconfigured. `task check` runs a full
  backend and frontend static, lint, and compile check.

See [docs/architecture.md](docs/architecture.md) for the request lifecycle,
streaming protocol, agent loop, and data model.

Tech stack:

- [templ](https://templ.guide) - HTML templating
- [Datastar](https://data-star.dev/) - streaming UI over SSE
- [tailwindcss](https://tailwindcss.com/) - CSS styling
- [Lit](https://lit.dev/) - web components
- [openai-go](https://github.com/openai/openai-go) - OpenAI-compatible chat completions
- [markdown-it](https://github.com/markdown-it/markdown-it) + [highlight.js](https://highlightjs.org/) - markdown rendering
- [go-chi](https://github.com/go-chi/chi) - HTTP router
- [goose](https://github.com/pressly/goose) - database migrations
- [sqlc](https://sqlc.dev/) - type-safe database queries
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) - pure Go SQLite driver

## Web assets

`web/build.ts` (run automatically by `task dev` on any `.ts` change) produces
two kinds of bundles:

- **Shared bundle:** everything in `web/lib/**/*.ts` becomes a single
  `static/bundle.js`. This is where web components live, and it is loaded
  synchronously in `<head>` so all custom elements are registered before the
  body parses.
- **Page bundles:** a `.ts` file in any other `web/` subdirectory (e.g.
  `web/root/root.ts`, next to its `.templ`) becomes its own bundle at
  `static/page-files/<dir>/<name>.js`. Load it on just that page:

  ```templ
  <script src={ chatwego.StaticRootPath("static/bundle.js") }></script>
  <script defer src={ chatwego.StaticRootPath("static/page-files/root/root.js") }></script>
  ```

Page files are plain top-level browser code, no exports needed. Avoid
importing from `lib/`: the iife bundle has no code splitting, so each import
duplicates code into the page bundle. Small utility imports are fine if you
accept that. Don't import web components from `lib/`: re-registering an
already-defined custom element throws.

Images and other files served as-is live in `web/assets/`, committed to git;
subdirectories welcome. `web/build.ts` hardlinks every file there into
`web/static/assets/`, mirroring the directory structure (a hardlink, not a
copy, so the repo keeps a single on-disk copy). The `task dev` watcher picks up
asset edits (images, svgs, fonts, favicons) and re-runs the build
automatically.

Reference them in templ with the same helper the bundles use:

```templ
<img src={ chatwego.StaticRootPath("static/assets/logo.svg") } alt="Logo"/>
```

`StaticRootPath` returns a plain root-relative path in dev and a content-hashed
URL in production (see below), so it works for `src`, `href`, and inline
`style` backgrounds alike.

- Production builds embed the assets into the binary (`//go:embed web/static/*`)
  and `hashfs` content-hashes the URLs for cache busting; dev mode serves them
  straight from disk with `no-store`.
- Don't put files in `web/static/` by hand as it is build output, gitignored and
  wiped on every `task build.go`.
- For icons that should scale or restyle with the theme, use web-component SVG
  icons in `web/lib/web-components/icons` (see AGENTS.md); `web/assets/` is for
  images and other static files.

## Database

Migrations live in `db/migrations/` (goose format), queries in
`db/queries/*.sql` (sqlc format). To change the schema:

1. Add a migration file to `db/migrations/` (e.g. `20240101120000_add_posts.sql`)
2. `task db.migrate` applies it to your local `data/sqlite.db`
3. Add or update queries in `db/queries/`
4. `task gen.db` regenerates type-safe Go code into `db/generated/sqlc/`

Migrations also run automatically on startup when `DB_MIGRATE_ON_START=true`,
which `task dev` sets by default.

## Adding tools and skills

Tools live in `lib/agent/tools`, one file per tool. Each file registers itself
into `tools.All` by declaring a package-level `var _ = register(tools.Tool{...})`,
so adding a tool means adding a file, not editing a list. The agent advertises
every registered tool to the model, and `/admin` lists it.

Skills live in `lib/agent/skills` as markdown files with YAML frontmatter:

```markdown
---
name: example
description: What this skill is for and when the model should fetch it.
---

The instructions handed back to the model when it fetches this skill.
```

Only the name and description go into the system prompt; the body is fetched on
demand through the `skill` tool. Skills are embedded into the binary, so adding
one means dropping in a `.md` file. `/admin` lists them too.

## Linting and checks

`task check` runs all checks in parallel: compile + golangci-lint for the go
code, and prettier + eslint + typescript + bundle build for the web code. The
git pre-push hook (installed by `task init`) runs `task check` as well.

## Project structure

```
.
├── .golangci.yml       go lint config
│
├── bin                 build output, gitignored
│
├── cmd
│   └── serve
│       ├── main.go     wiring, router, http server
│       └── handlers    HTTP handlers (index, chat, send-prompt, admin)
│
├── data                runtime sqlite data (gitignored inside)
│
├── db
│   ├── generated/sqlc  generated query code
│   ├── migrations      goose migrations
│   ├── queries         sqlc queries
│   ├── migrate.go      migrations runner
│   └── sqlc.yaml
│
├── docs
│   └── architecture.md request lifecycle, streaming, agent, data model
│
├── Dockerfile          production container image
│
├── files_embed.go      embeds web/static + migrations for production builds
│
├── files_noembed.go    serves web/static + migrations from disk in dev, noembed tag
│
├── .hooks              git hooks, symlinked by task init
│
├── lib                 shared go libraries
│   ├── agent           streaming agent, tools, skills
│   ├── datastar        server-side Datastar helpers
│   ├── env             environment config
│   ├── hashfs          content-hashed asset filesystem
│   ├── htmlclass       class attribute join helper
│   └── logging         slog handler
│
├── mise.toml           dev environment config
│
├── scripts
│   └── dev-run.sh      dev server runner
│
├── Taskfile.yml        taskfile with all tasks
│
├── tmp                 temp files/pidfiles, gitignored
│
└── web
    ├── admin           templ admin page (tools + skills)
    ├── assets          committed static assets (→ static/assets/)
    ├── build.ts        script to build web assets, outputs to web/static
    ├── lib             shared typescript libraries + web components (→ static/bundle.js)
    ├── root            templ root chat page (optional .ts → static/page-files/root/)
    └── static          build artifacts (bundle.js, page-files/, tailwind.css), gitignored
```

## Scaffolded from backwego-template

This project is scaffolded from
[backwego-template](https://github.com/Napam/backwego-template)

## License

[MIT](LICENSE)
