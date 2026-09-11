package handlers

import (
	"log/slog"
	"net/http"

	"github.com/Napam/chatwego/web/pages"
	"github.com/go-chi/chi/v5"
)

func (h *Handlers) GetChat(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	if _, err := h.queries.GetSessionById(r.Context(), sessionID); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	chatSessions, err := h.queries.ListChatSessions(r.Context())
	if err != nil {
		h.logger.Error("Failed to get sessions", slog.Any("error", err))
	}
	messages, err := h.queries.GetMessagesBySession(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("Failed to get messages", slog.Any("error", err))
	}
	if err := pages.RootPage(sessionID, chatSessions, messages).Render(r.Context(), w); err != nil {
		h.logger.Error("Render failed", slog.Any("error", err))
	}
}
