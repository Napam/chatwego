# ChatWeGo

A self-hosted chatbot. It streams markdown replies, shows reasoning traces and
tool calls, keeps a per-session sidebar, and talks to any OpenAI-compatible
chat completions API (llama.cpp, Ollama, vLLM, LM Studio, or OpenAI itself).

The server renders HTML with templ and streams updates with
[Datastar](https://data-star.dev/). SQLite stores identities, sessions, and
messages; goose manages migrations and sqlc generates the queries. Everything
compiles into a single binary. All tooling is managed by mise, and the live
reload development loop is set up and ready to go.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `OPENAI_BASE_URL` | `http://localhost:6700` | Base URL of the OpenAI-compatible API (the llama.cpp/llama-server default). |
| `OPENAI_MODEL` | `gpt-4o-mini` | Chat model to request. |
| `OPENAI_API_KEY` | unset | Sent as a bearer token when pointed at a provider that requires one. |
| `DB_PATH` | `./data/sqlite.db` | SQLite database file. |
| `HOST` / `PORT` | `localhost` / `8080` | Listen address. |
| `DB_MIGRATE_ON_START` | `false` | Run goose migrations on startup. |
| `COOKIE_SECURE` | `false` | Mark the session cookie `Secure` when serving over HTTPS. |

Requirements:

- [mise](https://mise.jdx.dev) for development environment setup - mise must be [activated](https://mise.jdx.dev/getting-started.html#activate-mise).

## Quickstart

1. Grab the repo via GitHub's "Use this template" button
   (gives you a fresh repo with no history).

   <details>
   <summary>Or you can git clone and reset the git state</summary>

   ```sh
   git clone --depth 1 https://github.com/Napam/chatwego my-app
   cd my-app
   rm -rf .git
   git init && git add -A && git commit -m "initial"
   ```

   </details>

2. Planning to use it for your own project? Run the rename wizard first:
   [Renaming](#renaming).

3. Run `task dev`. It sets up the git pre-push hook and starts the dev
   server with live reload.

4. Open the live reloading proxy at `localhost:7331`. The app itself listens on `PORT`
   (default `8080`).

5. Start making changes: the application entrypoint is `cmd/serve/main.go` and
   the frontend root page is `web/root/root.templ`.

For production build: `task build.go` (output in `bin/app`).

Docker:

```sh
task build.docker       # build image
task build.docker.run   # build and run
```

## Renaming

There's an interactive rename wizard:

`./scripts/rename.sh`

It replaces the Go module/package name, the display name, and the kebab-case
project name across all source files.

**Note:** This is a one-way operation. To undo, use `git checkout .`.

## How it works

The server renders HTML with templ. The composer POSTs to `/send-prompt` and
Datastar streams the reply back over SSE: each text delta is appended to a
hidden accumulator, and the `<markdown-message>` web component (markdown-it +
highlight.js) re-renders it live. Reasoning traces and tool calls render as
collapsible disclosures. The sidebar and session URL update as sessions are
created.

A visitor gets a cookie session on first request. All sessions and messages
are scoped to the resolved identity; the schema supports real logins, but the
current bootstrap always uses a system identity.

- Production builds embed web assets and migrations into the binary; dev mode
  serves them from disk, so changes show up on reload without a rebuild
  (details under [Web assets](#web-assets)).
- Web components with shadow DOM and shared TailwindCSS (and working dark mode
  toggling)
- Dockerfile that builds a minimal scratch image
- golangci-lint, eslint, and prettier preconfigured. `task check` runs a full
  backend+frontend static, lint, and compile check.

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

Migrations also run automatically on server startup (disable via
`DB_MIGRATE_ON_START=false`).

## Linting and checks

`task check` runs all checks in parallel: compile + golangci-lint for the go
code, and prettier + eslint + typescript + bundle build for the web code. The
git pre-push hook (installed by `task init`) runs `task check` as well.

## What next

You may want to:

- **Add real logins:** the identity/session tables and middleware are already
  in place; only the login flow is missing. Replace the system-identity
  bootstrap in `lib/session/session.go` with a real provider.
- **Add tools or skills:** drop a new tool in `lib/agent/tools` (self-registered
  via `tools.All`) or a markdown skill in `lib/agent/skills`. Both show up on
  `/admin` automatically.
- **Swap the database:** change the driver and connection string in
  `cmd/serve/main.go` to use Postgres, MySQL, or another driver, then update
  the sqlc config at `./db/sqlc.yaml`
- **Switch to JSON logging:** replace `logging.NewHandler(...)` with
  `slog.NewJSONHandler(os.Stdout, ...)` in `cmd/serve/main.go` for
  production-ready JSON logs

## Project structure

```
.
├── .golangci.yml       go lint config
│
├── bin                 build output, gitignored
│
├── cmd
│   └── serve           go entrypoint
│
├── data                runtime sqlite data (gitignored inside)
│
├── Dockerfile          production container image
│
├── db                  sqlc and migrations
│
├── files_embed.go      embeds web/static for production builds
│
├── files_noembed.go    serves web/static from disk in dev mode, noembed tag
│
├── .hooks              git hooks, symlinked by task init
│
├── lib                 shared go libraries
│
├── mise.toml           dev environment config
│
├── scripts
│   ├── dev-run.sh      dev server runner
│   └── rename.sh       project rename wizard
│
├── Taskfile.yml        taskfile with all tasks
│
├── tmp                 temp files/pidfiles, gitignored
│
└── web
    ├── assets          committed static assets (→ static/assets/)
    ├── build.ts        script to build web assets, outputs to web/static
    ├── lib             shared typescript libraries (→ static/bundle.js)
    ├── root            templ root page (optional .ts → static/page-files/root/)
    └── static          build artifacts (bundle.js, page-files/, tailwind.css), gitignored
```

## Who this is for

I've tried to keep the template nice to use for anybody, but I've mainly
optimized it for my own tools and workflows:

1. Neovim 0.12+ with my custom config
2. Being very CLI-first
3. UNIX first, I have no plans of supporting Windows. It may work out of the box
   anyways, I haven't tested.

## License

[MIT](LICENSE)
