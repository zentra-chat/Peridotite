package webhook

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/internal/utils"
)

type Handler struct {
	service *Service
}

const maxWebhookAvatarUploadBytes = 5 * 1024 * 1024

type WebhookSecretResponse struct {
	Webhook *models.Webhook `json:"webhook"`
	Token   string          `json:"token"`
	URL     string          `json:"url"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes(secret string) chi.Router {
	r := chi.NewRouter()

	// Authenticated management routes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(secret))
		r.Post("/channels/{channelId}", h.CreateWebhook)
		r.Post("/channels/{channelId}/avatar", h.UploadWebhookAvatar)
		r.Get("/channels/{channelId}", h.ListChannelWebhooks)
		r.Patch("/{webhookId}", h.UpdateWebhook)
		r.Post("/{webhookId}/rotate", h.RotateWebhookToken)
		r.Delete("/{webhookId}", h.DeleteWebhook)
	})

	// Public incoming endpoint.
	r.Post("/{webhookId}/{token}", h.ExecuteWebhook)

	return r
}

func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
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

	var req CreateWebhookRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	webhook, token, err := h.service.CreateWebhook(r.Context(), channelID, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage webhooks for this channel")
		case errors.Is(err, ErrChannelNotFound):
			utils.RespondError(w, http.StatusNotFound, "Channel not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to create webhook")
		}
		return
	}

	utils.RespondCreated(w, WebhookSecretResponse{
		Webhook: webhook,
		Token:   token,
		URL:     buildWebhookURL(r, webhook.ID, token),
	})
}

func (h *Handler) UploadWebhookAvatar(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookAvatarUploadBytes)
	if err := r.ParseMultipartForm(maxWebhookAvatarUploadBytes + 4096); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			utils.RespondError(w, http.StatusRequestEntityTooLarge, "File too large (max 5MB)")
			return
		}
		utils.RespondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	url, err := h.service.UploadWebhookAvatar(r.Context(), channelID, userID, file, header)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage webhooks for this channel")
		case errors.Is(err, ErrWebhookAvatarTooLarge):
			utils.RespondError(w, http.StatusRequestEntityTooLarge, "File too large (max 5MB)")
		case errors.Is(err, ErrInvalidWebhookAvatar):
			utils.RespondError(w, http.StatusBadRequest, "Invalid file type (images only)")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to upload webhook avatar")
		}
		return
	}

	utils.RespondSuccess(w, map[string]string{"url": url})
}

func (h *Handler) ListChannelWebhooks(w http.ResponseWriter, r *http.Request) {
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

	webhooks, err := h.service.ListChannelWebhooks(r.Context(), channelID, userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage webhooks for this channel")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to list webhooks")
		}
		return
	}

	utils.RespondSuccess(w, webhooks)
}

func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid webhook ID")
		return
	}

	var req UpdateWebhookRequest
	if err := utils.DecodeJSON(r, &req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	webhook, err := h.service.UpdateWebhook(r.Context(), webhookID, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage this webhook")
		case errors.Is(err, ErrChannelNotFound):
			utils.RespondError(w, http.StatusNotFound, "Target channel not found")
		case errors.Is(err, ErrWebhookNotFound):
			utils.RespondError(w, http.StatusNotFound, "Webhook not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to update webhook")
		}
		return
	}

	utils.RespondSuccess(w, webhook)
}

func (h *Handler) RotateWebhookToken(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid webhook ID")
		return
	}

	webhook, token, err := h.service.RotateWebhookToken(r.Context(), webhookID, userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage this webhook")
		case errors.Is(err, ErrWebhookNotFound):
			utils.RespondError(w, http.StatusNotFound, "Webhook not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to rotate webhook token")
		}
		return
	}

	utils.RespondSuccess(w, WebhookSecretResponse{
		Webhook: webhook,
		Token:   token,
		URL:     buildWebhookURL(r, webhook.ID, token),
	})
}

func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.RequireAuth(r.Context())
	if err != nil {
		utils.RespondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid webhook ID")
		return
	}

	if err := h.service.DeleteWebhook(r.Context(), webhookID, userID); err != nil {
		switch {
		case errors.Is(err, ErrWebhookInsufficientPerms):
			utils.RespondError(w, http.StatusForbidden, "Cannot manage this webhook")
		case errors.Is(err, ErrWebhookNotFound):
			utils.RespondError(w, http.StatusNotFound, "Webhook not found")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to delete webhook")
		}
		return
	}

	utils.RespondNoContent(w)
}

func (h *Handler) ExecuteWebhook(w http.ResponseWriter, r *http.Request) {
	webhookID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid webhook ID")
		return
	}

	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		utils.RespondError(w, http.StatusBadRequest, "Webhook token is required")
		return
	}

	bodyReader := http.MaxBytesReader(w, r.Body, MaxPayloadBytes)
	defer bodyReader.Close()

	rawBody, err := io.ReadAll(bodyReader)
	if err != nil {
		utils.RespondError(w, http.StatusRequestEntityTooLarge, "Webhook payload is too large")
		return
	}

	messageResp, err := h.service.ExecuteWebhook(r.Context(), webhookID, token, r.Header, r.Header.Get("Content-Type"), rawBody)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebhookNotFound), errors.Is(err, ErrInvalidWebhookToken):
			utils.RespondError(w, http.StatusNotFound, "Webhook not found")
		case errors.Is(err, ErrWebhookInactive):
			utils.RespondError(w, http.StatusGone, "Webhook is inactive")
		default:
			utils.RespondError(w, http.StatusInternalServerError, "Failed to process webhook")
		}
		return
	}

	utils.RespondSuccess(w, map[string]any{
		"messageId": messageResp.ID,
		"channelId": messageResp.ChannelID,
	})
}

func buildWebhookURL(r *http.Request, webhookID uuid.UUID, token string) string {
	proto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}

	return fmt.Sprintf("%s://%s/api/v1/webhooks/%s/%s", proto, host, webhookID.String(), token)
}

func firstForwardedValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, ",")
	return strings.TrimSpace(parts[0])
}
