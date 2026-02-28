package message

import (
	"net/http"
	"strconv"

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

	// Channel-scoped message routes
	r.Route("/channels/{channelId}/messages", func(r chi.Router) {
		r.Get("/", h.GetChannelMessages)
		r.Post("/", h.CreateMessage)
		r.Get("/pinned", h.GetPinnedMessages)
		r.Get("/search", h.SearchMessages)
		r.Post("/typing", h.StartTyping)
	})

	// Message-specific routes
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.GetMessage)
		r.Patch("/", h.UpdateMessage)
		r.Delete("/", h.DeleteMessage)
		r.Post("/pin", h.PinMessage)
		r.Delete("/pin", h.UnpinMessage)

		// Reactions
		r.Post("/reactions", h.AddReaction)
		r.Delete("/reactions/{emoji}", h.RemoveReaction)
	})

	return r
}

func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	channelID, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	var req CreateMessageRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	message, err := h.service.CreateMessage(r.Context(), channelID, userID, &req)
	if err != nil {
		switch err {
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot send messages in this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to create message")
		}
		return
	}

	utils.RespondCreated(w, message)
}

func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	message, err := h.service.GetMessage(r.Context(), messageID, userID)
	if err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot access this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to get message")
		}
		return
	}

	utils.RespondSuccess(w, message)
}

func (h *Handler) GetChannelMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	channelID, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	params := &GetMessagesParams{}

	if before := r.URL.Query().Get("before"); before != "" {
		if id, err := uuid.Parse(before); err == nil {
			params.Before = &id
		}
	}

	if after := r.URL.Query().Get("after"); after != "" {
		if id, err := uuid.Parse(after); err == nil {
			params.After = &id
		}
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			params.Limit = l
		}
	}

	messages, err := h.service.GetChannelMessages(r.Context(), channelID, userID, params)
	if err != nil {
		switch err {
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot access this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to get messages")
		}
		return
	}

	utils.RespondSuccess(w, messages)
}

func (h *Handler) UpdateMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	var req UpdateMessageRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	message, err := h.service.UpdateMessage(r.Context(), messageID, userID, &req)
	if err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrNotMessageOwner:
			utils.RespondError(w, http.StatusForbidden, "Cannot edit this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to update message")
		}
		return
	}

	utils.RespondSuccess(w, message)
}

func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	// Fetch message to get channelID
	msgResponse, err := h.service.GetMessage(r.Context(), messageID, userID)
	if err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot access this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to fetch message for deletion")
		}
		return
	}

	hasModPerm := h.service.CanManageMessages(r.Context(), msgResponse.ChannelID, userID)

	if err := h.service.DeleteMessage(r.Context(), messageID, userID, hasModPerm); err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot delete this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to delete message")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) AddReaction(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	var req struct {
		Emoji string `json:"emoji" validate:"required"`
	}
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.service.AddReaction(r.Context(), messageID, userID, req.Emoji); err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInvalidReaction:
			utils.RespondError(w, http.StatusBadRequest, "Invalid emoji")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot react to this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to add reaction")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	emoji := chi.URLParam(r, "emoji")
	if emoji == "" {
		utils.RespondError(w, http.StatusBadRequest, "Emoji is required")
		return
	}

	if err := h.service.RemoveReaction(r.Context(), messageID, userID, emoji); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to remove reaction")
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) PinMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	if err := h.service.PinMessage(r.Context(), messageID, userID, true); err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot pin messages in this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to pin message")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) UnpinMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	messageID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	if err := h.service.PinMessage(r.Context(), messageID, userID, false); err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot unpin messages in this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to unpin message")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) GetPinnedMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	channelID, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	messages, err := h.service.GetPinnedMessages(r.Context(), channelID, userID)
	if err != nil {
		switch err {
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot access this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to get pinned messages")
		}
		return
	}

	utils.RespondSuccess(w, messages)
}

func (h *Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	channelID, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		utils.RespondError(w, http.StatusBadRequest, "Search query is required")
		return
	}

	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	messages, err := h.service.SearchMessages(r.Context(), channelID, userID, query, limit)
	if err != nil {
		switch err {
		case ErrInsufficientPerms:
			utils.RespondError(w, http.StatusForbidden, "Cannot access this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to search messages")
		}
		return
	}

	utils.RespondSuccess(w, messages)
}

func (h *Handler) StartTyping(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	channelID, err := uuid.Parse(chi.URLParam(r, "channelId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	if err := h.service.SetTyping(r.Context(), channelID, userID); err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to set typing indicator")
		return
	}

	utils.RespondNoContent(w)
}
