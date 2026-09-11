package handlers

import (
	"database/sql"
	"log/slog"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent"
)

type Handlers struct {
	logger  *slog.Logger
	db      *sql.DB
	queries *sqlc.Queries
	agent   *agent.Agent
}

func New(logger *slog.Logger, db *sql.DB, queries *sqlc.Queries, chatAgent *agent.Agent) *Handlers {
	return &Handlers{logger: logger, db: db, queries: queries, agent: chatAgent}
}
