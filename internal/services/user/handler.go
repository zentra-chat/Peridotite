package user

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/models"
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

	// Current user routes
	r.Get("/me", h.GetCurrentUser)
	r.Get("/me/id", h.GetCurrentUserID)
	r.Patch("/me", h.UpdateProfile)
	r.Delete("/me/avatar", h.RemoveAvatar)
	r.Get("/me/settings", h.GetSettings)
	r.Patch("/me/settings", h.UpdateSettings)
	r.Put("/me/status", h.UpdateStatus)

	// User lookup routes
	r.Get("/search", h.SearchUsers)
	r.Get("/{id}", h.GetUser)
	r.Get("/username/{username}", h.GetUserByUsername)

	// Block management
	r.Get("/me/blocks", h.GetBlockedUsers)
	r.Post("/me/blocks/{id}", h.BlockUser)
	r.Delete("/me/blocks/{id}", h.UnblockUser)

	return r
}

func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.service.GetUserByID(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	utils.RespondSuccess(w, user)
}

func (h *Handler) GetCurrentUserID(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	utils.RespondSuccess(w, map[string]string{
		"id": userID.String(),
	})
}

func (h *Handler) RemoveAvatar(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.service.RemoveAvatar(r.Context(), userID); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to remove avatar")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	user, err := h.service.GetPublicUser(r.Context(), id)
	if err != nil {
		if err == ErrUserNotFound {
			utils.RespondError(w, http.StatusNotFound, "User not found")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	utils.RespondSuccess(w, user)
}

func (h *Handler) GetUserByUsername(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if username == "" {
		utils.RespondError(w, http.StatusBadRequest, "Username is required")
		return
	}

	user, err := h.service.GetUserByUsername(r.Context(), username)
	if err != nil {
		if err == ErrUserNotFound {
			utils.RespondError(w, http.StatusNotFound, "User not found")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get user")
		return
	}

	utils.RespondSuccess(w, user)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req UpdateProfileRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	user, err := h.service.UpdateProfile(r.Context(), userID, &req)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to update profile")
		return
	}

	utils.RespondSuccess(w, user)
}

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=online away busy invisible offline"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	if err := h.service.UpdateStatus(r.Context(), userID, models.UserStatus(req.Status)); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to update status")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	query := utils.GetQueryString(r, "q", "")
	if query == "" {
		utils.RespondError(w, http.StatusBadRequest, "Search query is required")
		return
	}

	page := utils.GetQueryInt(r, "page", 1)
	pageSize := utils.GetQueryInt(r, "pageSize", 20)
	offset := (page - 1) * pageSize

	users, total, err := h.service.SearchUsers(r.Context(), query, pageSize, offset)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to search users")
		return
	}

	utils.RespondPaginated(w, users, total, page, pageSize)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	settings, err := h.service.GetSettings(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get settings")
		return
	}

	utils.RespondSuccess(w, settings)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req UpdateSettingsRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	settings, err := h.service.UpdateSettings(r.Context(), userID, &req)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to update settings")
		return
	}

	utils.RespondSuccess(w, settings)
}

func (h *Handler) GetBlockedUsers(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	users, err := h.service.GetBlockedUsers(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get blocked users")
		return
	}

	utils.RespondSuccess(w, users)
}

func (h *Handler) BlockUser(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	blockedID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.service.BlockUser(r.Context(), userID, blockedID); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to block user")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) UnblockUser(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	blockedID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.service.UnblockUser(r.Context(), userID, blockedID); err != nil {
		if err == ErrNotBlocked {
			utils.RespondError(w, http.StatusBadRequest, "User is not blocked")
			return
		}
		utils.RespondError(w, http.StatusInternalServerError, "Failed to unblock user")
		return
	}

	utils.RespondNoContent(w)
}
