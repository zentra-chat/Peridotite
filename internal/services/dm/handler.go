package dm

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

	r.Route("/conversations", func(r chi.Router) {
		r.Get("/", h.ListConversations)
		r.Post("/", h.CreateConversation)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.GetConversation)
			r.Post("/read", h.MarkRead)
			r.Get("/messages", h.GetMessages)
			r.Post("/messages", h.SendMessage)
		})
	})

	r.Route("/messages/{id}", func(r chi.Router) {
		r.Patch("/", h.UpdateMessage)
		r.Delete("/", h.DeleteMessage)
	})

	return r
}

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	conversations, err := h.service.ListConversations(r.Context(), userID)
	if err != nil {
		utils.RespondError(w, http.StatusInternalServerError, "Failed to load conversations")
		return
	}

	utils.RespondSuccess(w, conversations)
}

func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req CreateConversationRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	conversation, err := h.service.CreateOrGetConversation(r.Context(), userID, req.UserID)
	if err != nil {
		switch err {
		case ErrBlocked:
			utils.RespondError(w, http.StatusForbidden, "Cannot message this user")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to create conversation")
		}
		return
	}

	utils.RespondCreated(w, conversation)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid conversation ID")
		return
	}

	conversation, err := h.service.GetConversation(r.Context(), conversationID, userID)
	if err != nil {
		switch err {
		case ErrConversationNotFound:
			utils.RespondError(w, http.StatusNotFound, "Conversation not found")
		case ErrNotParticipant:
			utils.RespondError(w, http.StatusForbidden, "Not a participant")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to get conversation")
		}
		return
	}

	utils.RespondSuccess(w, conversation)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid conversation ID")
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

	messages, err := h.service.GetMessages(r.Context(), conversationID, userID, params)
	if err != nil {
		switch err {
		case ErrNotParticipant:
			utils.RespondError(w, http.StatusForbidden, "Not a participant")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to get messages")
		}
		return
	}

	utils.RespondSuccess(w, messages)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid conversation ID")
		return
	}

	var req SendMessageRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.Validate(&req); err != nil {
		utils.RespondValidationError(w, utils.FormatValidationErrors(err))
		return
	}

	message, err := h.service.SendMessage(r.Context(), conversationID, userID, &req)
	if err != nil {
		switch err {
		case ErrNotParticipant:
			utils.RespondError(w, http.StatusForbidden, "Not a participant")
		case ErrInvalidAttachment:
			utils.RespondError(w, http.StatusBadRequest, "Invalid attachment")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to send message")
		}
		return
	}

	utils.RespondCreated(w, message)
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

	if err := h.service.DeleteMessage(r.Context(), messageID, userID); err != nil {
		switch err {
		case ErrMessageNotFound:
			utils.RespondError(w, http.StatusNotFound, "Message not found")
		case ErrNotMessageOwner:
			utils.RespondError(w, http.StatusForbidden, "Cannot delete this message")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to delete message")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	conversationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid conversation ID")
		return
	}

	if err := h.service.MarkRead(r.Context(), conversationID, userID); err != nil {
		switch err {
		case ErrNotParticipant:
			utils.RespondError(w, http.StatusForbidden, "Not a participant")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to mark read")
		}
		return
	}

	utils.RespondNoContent(w)
}
