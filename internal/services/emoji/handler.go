package emoji

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/utils"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Get all custom emojis the user can access (across all their communities)
	r.Get("/", h.GetAccessibleEmojis)

	// Resolve a single emoji by ID (for rendering in messages)
	r.Get("/{id}", h.GetEmoji)

	// Community-scoped emoji management
	r.Route("/communities/{communityId}", func(r chi.Router) {
		r.Get("/", h.GetCommunityEmojis)
		r.Post("/", h.CreateEmoji)
	})

	// Single emoji operations
	r.Patch("/{id}", h.UpdateEmoji)
	r.Delete("/{id}", h.DeleteEmoji)

	return r
}

// GetAccessibleEmojis returns every custom emoji the user can use (from all their communities)
func (h *Handler) GetAccessibleEmojis(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	emojis, err := h.service.GetAllAccessibleEmojis(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch emojis")
		return
	}

	utils.RespondSuccess(w, emojis)
}

// GetEmoji resolves a single emoji by ID
func (h *Handler) GetEmoji(w http.ResponseWriter, r *http.Request) {
	emojiID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid emoji ID")
		return
	}

	emoji, err := h.service.ResolveEmoji(r.Context(), emojiID)
	if err != nil {
		if err == ErrEmojiNotFound {
			utils.RespondError(w, http.StatusNotFound, "Emoji not found")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch emoji")
		return
	}

	utils.RespondSuccess(w, emoji)
}

// GetCommunityEmojis lists all emojis belonging to a specific community
func (h *Handler) GetCommunityEmojis(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	communityID, err := uuid.Parse(chi.URLParam(r, "communityId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid community ID")
		return
	}

	emojis, err := h.service.GetCommunityEmojis(r.Context(), communityID, userID)
	if err != nil {
		if err == ErrNotMember {
			utils.RespondError(w, http.StatusForbidden, "Not a member of this community")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch emojis")
		return
	}

	utils.RespondSuccess(w, emojis)
}

// CreateEmoji uploads a new custom emoji for a community
func (h *Handler) CreateEmoji(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	communityID, err := uuid.Parse(chi.URLParam(r, "communityId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid community ID")
		return
	}

	// Limit upload size
	r.Body = http.MaxBytesReader(w, r.Body, MaxEmojiSize+4096) // extra room for form fields
	if err := r.ParseMultipartForm(MaxEmojiSize + 4096); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Request too large")
		return
	}

	name := r.FormValue("name")
	if name == "" {
		utils.RespondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Image file is required")
		return
	}
	defer file.Close()

	emoji, err := h.service.CreateEmoji(r.Context(), communityID, userID, name, file, header)
	if err != nil {
		switch err {
		case ErrInvalidName:
			utils.RespondError(w, http.StatusBadRequest, err.Error())
		case ErrNameTaken:
			utils.RespondError(w, http.StatusConflict, err.Error())
		case ErrInvalidImage:
			utils.RespondError(w, http.StatusBadRequest, err.Error())
		case ErrImageTooLarge:
			utils.RespondError(w, http.StatusBadRequest, err.Error())
		case ErrTooManyEmojis:
			utils.RespondError(w, http.StatusBadRequest, err.Error())
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "You don't have permission to manage emojis")
		case ErrNotMember:
			utils.RespondError(w, http.StatusForbidden, "Not a member of this community")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to create emoji")
		}
		return
	}

	utils.RespondCreated(w, emoji)
}

// UpdateEmoji renames an existing emoji
func (h *Handler) UpdateEmoji(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	emojiID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid emoji ID")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	emoji, err := h.service.UpdateEmoji(r.Context(), emojiID, userID, req.Name)
	if err != nil {
		switch err {
		case ErrEmojiNotFound:
			utils.RespondError(w, http.StatusNotFound, "Emoji not found")
		case ErrInvalidName:
			utils.RespondError(w, http.StatusBadRequest, err.Error())
		case ErrNameTaken:
			utils.RespondError(w, http.StatusConflict, err.Error())
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "You don't have permission to manage emojis")
		case ErrNotMember:
			utils.RespondError(w, http.StatusForbidden, "Not a member of this community")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to update emoji")
		}
		return
	}

	utils.RespondSuccess(w, emoji)
}

// DeleteEmoji removes a custom emoji
func (h *Handler) DeleteEmoji(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	emojiID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid emoji ID")
		return
	}

	if err := h.service.DeleteEmoji(r.Context(), emojiID, userID); err != nil {
		switch err {
		case ErrEmojiNotFound:
			utils.RespondError(w, http.StatusNotFound, "Emoji not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "You don't have permission to manage emojis")
		case ErrNotMember:
			utils.RespondError(w, http.StatusForbidden, "Not a member of this community")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to delete emoji")
		}
		return
	}

	utils.RespondNoContent(w)
}
