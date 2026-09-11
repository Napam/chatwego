package handlers

import (
	"log/slog"
	"net/http"

	"github.com/Napam/chatwego/web/pages"
)

// Admin renders a read-only page listing every tool and skill loaded into the
// agent. Both are static, code-defined registries (see tools.All,
// skills.Sorted), so no database access is needed here.
func (h *Handlers) Admin(w http.ResponseWriter, r *http.Request) {
	if err := pages.AdminPage().Render(r.Context(), w); err != nil {
		h.logger.Error("Render failed", slog.Any("error", err))
	}
}
