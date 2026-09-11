package handlers

import (
	"log/slog"
	"net/http"

	"github.com/Napam/chatwego/lib/session"
	"github.com/Napam/chatwego/web/root"
	"github.com/go-chi/chi/v5"
)

func (h *Handlers) GetChat(w http.ResponseWriter, r *http.Request) {
	identityID := session.IdentityFromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")

	chatSession, err := h.queries.GetSessionById(r.Context(), sessionID)
	if err != nil || chatSession.IdentityID != identityID {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	chatSessions, err := h.queries.GetSessionsByOwner(r.Context(), identityID)
	if err != nil {
		h.logger.Error("Failed to get sessions", slog.Any("error", err))
	}
	messages, err := h.queries.GetMessagesBySession(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("Failed to get messages", slog.Any("error", err))
	}
	if err := root.RootPage(sessionID, chatSessions, messages).Render(r.Context(), w); err != nil {
		h.logger.Error("Render failed", slog.Any("error", err))
	}
}
