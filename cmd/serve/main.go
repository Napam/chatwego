package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Napam/chatwego"
	"github.com/Napam/chatwego/cmd/serve/handlers"
	"github.com/Napam/chatwego/db"
	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent"
	"github.com/Napam/chatwego/lib/env"
	"github.com/Napam/chatwego/lib/logging"
	"github.com/Napam/chatwego/lib/session"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	_ "modernc.org/sqlite"
)

func main() {
	handler := logging.NewHandler(&slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

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

	dbConn, err := sql.Open("sqlite", env.Vars.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
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

	if err := http.ListenAndServe(env.Vars.Host+":"+env.Vars.Port, router); err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
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
	router.Use(session.Middleware(logger, dbQueries))

	chatwego.SetupStatic(router)

	h := handlers.New(logger, dbQueries, chatAgent)

	// === ROUTES ===

	router.Get("/", h.Index)
	router.Get("/chats/{sessionID}", h.GetChat)
	router.Post("/send-prompt", h.SendPrompt)
	router.Get("/admin", h.Admin)

	return router
}
