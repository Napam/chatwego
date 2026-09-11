package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Napam/chatwego"
	"github.com/Napam/chatwego/cmd/serve/handlers"
	"github.com/Napam/chatwego/db"
	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent"
	"github.com/Napam/chatwego/lib/env"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	_ "modernc.org/sqlite"
)

func main() {
	// Plain JSON on stdout: `task dev` pipes it into hl for the human-readable
	// view, and elsewhere it stays machine-parseable.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("Server exited with error", slog.Any("error", err))
		os.Exit(1)
	}
}

// run wires up dependencies and starts the HTTP server, returning any fatal
// startup or serving error so main stays a one-liner.
func run(logger *slog.Logger) error {
	if cwd, err := os.Getwd(); err == nil {
		logger.Info("Initializing chatwego", slog.String("cwd", cwd))
	}

	dbConn, err := openSQLite(env.Vars.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dbConn.Close(); err != nil {
			logger.Error("Failed to close database", slog.Any("error", err))
		}
	}()

	if env.Vars.DbMigrateOnStart {
		if _, err := db.RunMigrations(
			context.Background(),
			chatwego.MigrationsFS,
			dbConn,
			logger,
		); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	openaiClient := openai.NewClient(option.WithBaseURL(env.Vars.OpenAIBaseURL))
	chatAgent := agent.New(openaiClient, env.Vars.OpenAIModel)

	router := SetupRouter(logger, dbConn, chatAgent)

	logAttrs := []slog.Attr{
		slog.String("address", env.Vars.Host+":"+env.Vars.Port),
		slog.String("db_path", env.Vars.DBPath),
		slog.String("openai_base_url", env.Vars.OpenAIBaseURL),
	}
	if env.Vars.LiveReloadHost != "" && env.Vars.LiveReloadPort != "" {
		logAttrs = append(logAttrs, slog.String(
			"live_reload_address",
			env.Vars.LiveReloadHost+":"+env.Vars.LiveReloadPort+" (you have to use 'task dev' for this to work)",
		))
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "Server running", logAttrs...)

	srv := &http.Server{
		Addr:    env.Vars.Host + ":" + env.Vars.Port,
		Handler: router,
		// No WriteTimeout: an SSE reply stays open for the whole model
		// response, and a write deadline would cut it off.
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

// sqlitePragmas configure every connection: foreign keys (off by default in
// SQLite), a busy timeout so concurrent writers wait instead of failing
// immediately, and WAL plus NORMAL sync so readers don't block the writer.
const sqlitePragmas = "_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

// openSQLite opens the database. The pragmas travel in the DSN so they apply
// to every pooled connection, not just the first.
func openSQLite(path string) (*sql.DB, error) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	dbConn, err := sql.Open("sqlite", path+separator+sqlitePragmas)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return dbConn, nil
}

func SetupRouter(logger *slog.Logger, dbConn *sql.DB, chatAgent *agent.Agent) *chi.Mux {
	router := chi.NewRouter()

	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			logger.Info("HTTP Request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.String("ip", r.RemoteAddr),
				slog.Duration("duration", time.Since(start)),
			)
		})
	})
	router.Use(middleware.Recoverer)

	dbQueries := sqlc.New(dbConn)

	chatwego.SetupStatic(router)

	h := handlers.New(logger, dbConn, dbQueries, chatAgent)

	// === ROUTES ===

	router.Get("/", h.Index)
	router.Get("/chats/{sessionID}", h.GetChat)
	router.Post("/send-prompt", h.SendPrompt)
	router.Get("/admin", h.Admin)

	return router
}
