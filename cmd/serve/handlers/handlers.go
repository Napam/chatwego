package handlers

import (
	"log/slog"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/agent"
)

type Handlers struct {
	logger  *slog.Logger
	queries *sqlc.Queries
	agent   *agent.Agent
}

func New(logger *slog.Logger, queries *sqlc.Queries, chatAgent *agent.Agent) *Handlers {
	return &Handlers{logger: logger, queries: queries, agent: chatAgent}
}
