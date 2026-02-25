package notification

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/utils"
)

// Handler exposes notification endpoints.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Routes returns the chi router for notification endpoints.
// Mount at /notifications (under the authenticated group).
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListNotifications)
	r.Get("/unread-count", h.GetUnreadCount)
	r.Post("/read-all", h.MarkAllRead)

	r.Route("/{id}", func(r chi.Router) {
		r.Post("/read", h.MarkRead)
		r.Delete("/", h.DeleteNotification)
	})

	// Mentions for a specific message (useful for the client to render mention badges)
	r.Get("/messages/{messageId}/mentions", h.GetMessageMentions)

	return r
}

// GET /notifications?page=1&limit=50
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	notifications, total, err := h.service.GetNotifications(r.Context(), userID, limit, offset)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch notifications")
		return
	}

	utils.RespondPaginated(w, notifications, total, page, limit)
}

// GET /notifications/unread-count
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	count, err := h.service.GetUnreadCount(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to get unread count")
		return
	}

	utils.RespondSuccess(w, map[string]int64{"count": count})
}

// POST /notifications/{id}/read
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	notifID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}

	if err := h.service.MarkRead(r.Context(), notifID, userID); err != nil {
		switch err {
		case ErrNotFound:
			utils.RespondError(w, http.StatusNotFound, "Notification not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to mark notification as read")
		}
		return
	}

	utils.RespondNoContent(w)
}

// POST /notifications/read-all
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := h.service.MarkAllRead(r.Context(), userID); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to mark all notifications as read")
		return
	}

	utils.RespondNoContent(w)
}

// DELETE /notifications/{id}
func (h *Handler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	notifID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}

	if err := h.service.DeleteNotification(r.Context(), notifID, userID); err != nil {
		switch err {
		case ErrNotFound:
			utils.RespondError(w, http.StatusNotFound, "Notification not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to delete notification")
		}
		return
	}

	utils.RespondNoContent(w)
}

// GET /notifications/messages/{messageId}/mentions
func (h *Handler) GetMessageMentions(w http.ResponseWriter, r *http.Request) {
	_, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "messageId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	mentions, err := h.service.GetMessageMentions(r.Context(), messageID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch mentions")
		return
	}

	utils.RespondSuccess(w, mentions)
}
