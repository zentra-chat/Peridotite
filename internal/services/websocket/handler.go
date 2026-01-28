package websocket

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/utils"
	"github.com/zentra/peridotite/pkg/auth"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking in production
		return true
	},
}

type Handler struct {
	hub       *Hub
	jwtSecret string
}

func NewHandler(hub *Hub, jwtSecret string) *Handler {
	return &Handler{
		hub:       hub,
		jwtSecret: jwtSecret,
	}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// WebSocket endpoint - prefers /ws but handles both /ws and /ws/
	r.Get("/", h.HandleWebSocket)

	// REST endpoints for presence/typing (alternative to WebSocket)
	// This should really be done via Websocket, but I am lazy.
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(h.jwtSecret))
		r.Get("/presence/{userId}", h.GetUserPresence)
		r.Get("/channels/{channelId}/typing", h.GetTypingUsers)
	})

	return r
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Get user ID from query parameter (token validation)
	token := r.URL.Query().Get("token")
	if token == "" {
		// Also check Authorization header
		token = r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
	}

	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Validate JWT token
	claims, err := auth.ValidateAccessToken(token, h.jwtSecret)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID in token", http.StatusUnauthorized)
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade WebSocket connection")
		return
	}

	// Create client
	client := NewClient(userID, conn, h.hub)

	// Register client with hub
	h.hub.register <- client

	// Send READY event
	client.SendEvent(&Event{
		Type: EventTypeReady,
		Data: map[string]interface{}{
			"clientId":  client.ID.String(),
			"userId":    userID.String(),
			"sessionId": client.ID.String(),
		},
	})

	// Start goroutines
	go client.WritePump()
	go client.ReadPump()
}

func (h *Handler) GetUserPresence(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	status := h.hub.GetUserPresence(r.Context(), userID)
	utils.RespondSuccess(w, map[string]interface{}{
		"userId": userID.String(),
		"status": status,
		"online": h.hub.IsUserOnline(userID),
	})
}

func (h *Handler) GetTypingUsers(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelId")
	if channelID == "" {
		utils.RespondError(w, http.StatusBadRequest, "Invalid channel ID")
		return
	}

	users := h.hub.GetTypingUsers(r.Context(), channelID)
	userStrings := make([]string, len(users))
	for i, u := range users {
		userStrings[i] = u.String()
	}

	utils.RespondSuccess(w, map[string]interface{}{
		"channelId": channelID,
		"users":     userStrings,
	})
}
