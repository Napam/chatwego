package handlers

import (
	"log/slog"
	"net/http"

	"github.com/Napam/chatwego/db/generated/sqlc"
	"github.com/Napam/chatwego/lib/session"
	"github.com/Napam/chatwego/web/root"
)

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	identityID := session.IdentityFromContext(r.Context())
	chatSessions, err := h.queries.GetSessionsByOwner(r.Context(), identityID)
	if err != nil {
		h.logger.Error("Failed to get sessions", slog.Any("error", err))
	}
	var messages []sqlc.ChatMessage
	if err := root.RootPage("", chatSessions, messages).Render(r.Context(), w); err != nil {
		h.logger.Error("Render failed", slog.Any("error", err))
	}
}
