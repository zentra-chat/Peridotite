package channeltype

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/internal/utils"
)

type Handler struct {
	registry *Registry
}

func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListChannelTypes)
	r.Get("/{typeId}", h.GetChannelType)
	return r
}

// ListChannelTypes returns all registered channel type definitions
func (h *Handler) ListChannelTypes(w http.ResponseWriter, r *http.Request) {
	_, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	types := h.registry.All()
	utils.RespondSuccess(w, types)
}

// GetChannelType returns a single channel type definition by ID
func (h *Handler) GetChannelType(w http.ResponseWriter, r *http.Request) {
	_, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	typeID := chi.URLParam(r, "typeId")
	def, err := h.registry.Get(typeID)
	if err != nil {
		if err == ErrTypeNotFound {
			utils.RespondError(w, http.StatusNotFound, "Channel type not found")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get channel type")
		return
	}

	utils.RespondSuccess(w, def)
}

// RegisterChannelTypeRequest is used by plugins to register a new channel type
type RegisterChannelTypeRequest struct {
	ID           string `json:"id" validate:"required,min=1,max=64"`
	Name         string `json:"name" validate:"required,min=1,max=128"`
	Description  string `json:"description"`
	Icon         string `json:"icon" validate:"required,min=1,max=64"`
	Capabilities int64  `json:"capabilities"`
	PluginID     string `json:"pluginId"`
}

// RegisterType handles plugin-initiated channel type registration.
// Not exposed as a public route yet, but ready for when the plugin system lands.
func (h *Handler) RegisterType(ctx *http.Request, req *RegisterChannelTypeRequest) error {
	def := &models.ChannelTypeDefinition{
		ID:           req.ID,
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Capabilities: req.Capabilities,
		BuiltIn:      false,
		PluginID:     &req.PluginID,
	}

	return h.registry.Register(ctx.Context(), def)
}
