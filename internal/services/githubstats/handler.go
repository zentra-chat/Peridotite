package githubstats

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zentra/server/internal/utils"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/stats", h.GetStats)
	return r
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetStats(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusBadGateway, "Failed to fetch GitHub stats")
		return
	}

	utils.RespondSuccess(w, stats)
}
