package handlers

import (
	"log/slog"
	"net/http"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/web/pages"
)

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	chatSessions, err := h.queries.ListChatSessions(r.Context())
	if err != nil {
		h.logger.Error("Failed to get sessions", slog.Any("error", err))
	}
	var messages []sqlc.ChatMessage
	if err := pages.RootPage("", chatSessions, messages).Render(r.Context(), w); err != nil {
		h.logger.Error("Render failed", slog.Any("error", err))
	}
}
