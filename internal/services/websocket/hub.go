package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/zentra/peridotite/internal/services/channel"
	"github.com/zentra/peridotite/internal/services/dm"
	"github.com/zentra/peridotite/internal/services/user"
	"github.com/zentra/peridotite/internal/services/voice"
)

// Event types
const (
	EventTypeMessage          = "MESSAGE_CREATE"
	EventTypeMessageUpdate    = "MESSAGE_UPDATE"
	EventTypeMessageDelete    = "MESSAGE_DELETE"
	EventTypeTypingStart      = "TYPING_START"
	EventTypePresenceUpdate   = "PRESENCE_UPDATE"
	EventTypeChannelCreate    = "CHANNEL_CREATE"
	EventTypeChannelUpdate    = "CHANNEL_UPDATE"
	EventTypeChannelDelete    = "CHANNEL_DELETE"
	EventTypeMemberJoin       = "MEMBER_JOIN"
	EventTypeMemberLeave      = "MEMBER_LEAVE"
	EventTypeMemberUpdate     = "MEMBER_UPDATE"
	EventTypeReactionAdd      = "REACTION_ADD"
	EventTypeReactionRemove   = "REACTION_REMOVE"
	EventTypeVoiceState       = "VOICE_STATE_UPDATE"
	EventTypeVoiceJoin        = "VOICE_JOIN"
	EventTypeVoiceLeave       = "VOICE_LEAVE"
	EventTypeVoiceSignal      = "VOICE_SIGNAL"
	EventTypeCommunityUpdate  = "COMMUNITY_UPDATE"
	EventTypeUserUpdate       = "USER_UPDATE"
	EventTypeDMMessage        = "DM_MESSAGE_CREATE"
	EventTypeDMMessageUpdate  = "DM_MESSAGE_UPDATE"
	EventTypeDMMessageDelete  = "DM_MESSAGE_DELETE"
	EventTypeDMReactionAdd    = "DM_REACTION_ADD"
	EventTypeDMReactionRemove = "DM_REACTION_REMOVE"
	EventTypeReady            = "READY"
	EventTypeHeartbeat        = "HEARTBEAT"
	EventTypeHeartbeatAck     = "HEARTBEAT_ACK"
	EventTypeNotification     = "NOTIFICATION"
	EventTypeNotificationRead = "NOTIFICATION_READ"
)

// Client represents a WebSocket client connection
type Client struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Conn       *websocket.Conn
	Send       chan []byte
	Hub        *Hub
	Subscribed map[string]bool // Channel/community subscriptions
	mu         sync.RWMutex
	lastPing   time.Time
}

// Hub manages all WebSocket connections
type Hub struct {
	clients        map[uuid.UUID]*Client         // Client ID -> Client
	userClients    map[uuid.UUID][]*Client       // User ID -> Clients (user can have multiple connections)
	channels       map[string]map[uuid.UUID]bool // Channel ID -> Client IDs
	register       chan *Client
	unregister     chan *Client
	broadcast      chan *BroadcastMessage
	redis          *redis.Client
	channelService *channel.Service
	userService    *user.Service
	dmService      *dm.Service
	voiceService   *voice.Service
	mu             sync.RWMutex
}

// BroadcastMessage represents a message to be broadcast
type BroadcastMessage struct {
	ChannelID       string
	Event           *Event
	ExcludeClientID *uuid.UUID
}

// Event represents a WebSocket event
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ClientMessage represents an incoming message from a client
type ClientMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func NewHub(redisClient *redis.Client, channelService *channel.Service, userService *user.Service, dmService *dm.Service, voiceService *voice.Service) *Hub {
	return &Hub{
		clients:        make(map[uuid.UUID]*Client),
		userClients:    make(map[uuid.UUID][]*Client),
		channels:       make(map[string]map[uuid.UUID]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan *BroadcastMessage, 256),
		redis:          redisClient,
		channelService: channelService,
		userService:    userService,
		dmService:      dmService,
		voiceService:   voiceService,
	}
}

func (h *Hub) Run(ctx context.Context) {
	// Start Redis subscription for cross-server events
	go h.subscribeToRedis(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case msg := <-h.broadcast:
			h.broadcastToChannel(msg)
		}
	}
}

func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client.ID] = client
	h.userClients[client.UserID] = append(h.userClients[client.UserID], client)

	log.Info().
		Str("clientId", client.ID.String()).
		Str("userId", client.UserID.String()).
		Msg("WebSocket client connected")

	// Update presence
	h.setUserPresence(context.Background(), client.UserID, "online")
}

func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.ID]; ok {
		delete(h.clients, client.ID)
		close(client.Send)

		// Remove from user clients
		clients := h.userClients[client.UserID]
		for i, c := range clients {
			if c.ID == client.ID {
				h.userClients[client.UserID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}

		// If no more connections for this user, set offline and disconnect voice
		if len(h.userClients[client.UserID]) == 0 {
			delete(h.userClients, client.UserID)
			h.setUserPresence(context.Background(), client.UserID, "offline")

			// Disconnect from voice channels
			if h.voiceService != nil {
				channelIDs, _ := h.voiceService.DisconnectUser(context.Background(), client.UserID)
				for _, channelID := range channelIDs {
					h.broadcastToChannel(&BroadcastMessage{
						ChannelID: channelID.String(),
						Event: &Event{
							Type: EventTypeVoiceLeave,
							Data: map[string]interface{}{
								"channelId": channelID.String(),
								"userId":    client.UserID.String(),
							},
						},
					})
				}
			}
		}

		// Remove from channel subscriptions
		for channelID := range client.Subscribed {
			if clients, ok := h.channels[channelID]; ok {
				delete(clients, client.ID)
				if len(clients) == 0 {
					delete(h.channels, channelID)
				}
			}
		}

		log.Info().
			Str("clientId", client.ID.String()).
			Str("userId", client.UserID.String()).
			Msg("WebSocket client disconnected")
	}
}

func (h *Hub) broadcastToChannel(msg *BroadcastMessage) {
	data, err := json.Marshal(msg.Event)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal broadcast event")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// If ChannelID is empty, broadcast to all connected clients
	if msg.ChannelID == "" {
		for clientID, client := range h.clients {
			if msg.ExcludeClientID != nil && clientID == *msg.ExcludeClientID {
				continue
			}
			select {
			case client.Send <- data:
			default:
				log.Warn().Str("clientId", clientID.String()).Msg("Client send buffer full")
			}
		}
		return
	}

	clients, ok := h.channels[msg.ChannelID]
	if !ok {
		return
	}

	for clientID := range clients {
		if msg.ExcludeClientID != nil && clientID == *msg.ExcludeClientID {
			continue
		}
		if client, ok := h.clients[clientID]; ok {
			select {
			case client.Send <- data:
			default:
				// Client send buffer full, skip
				log.Warn().
					Str("clientId", clientID.String()).
					Msg("Client send buffer full")
			}
		}
	}
}

// Subscribe adds a client to a channel's broadcast list
func (h *Hub) Subscribe(client *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.channels[channelID] == nil {
		h.channels[channelID] = make(map[uuid.UUID]bool)
	}
	h.channels[channelID][client.ID] = true

	client.mu.Lock()
	client.Subscribed[channelID] = true
	client.mu.Unlock()

	log.Debug().
		Str("clientId", client.ID.String()).
		Str("channelId", channelID).
		Msg("Client subscribed to channel")
}

// Unsubscribe removes a client from a channel's broadcast list
func (h *Hub) Unsubscribe(client *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.channels[channelID]; ok {
		delete(clients, client.ID)
		if len(clients) == 0 {
			delete(h.channels, channelID)
		}
	}

	client.mu.Lock()
	delete(client.Subscribed, channelID)
	client.mu.Unlock()
}

// Broadcast sends an event to all clients subscribed to a channel
func (h *Hub) Broadcast(channelID string, event *Event, excludeClientID *uuid.UUID) {
	h.broadcast <- &BroadcastMessage{
		ChannelID:       channelID,
		Event:           event,
		ExcludeClientID: excludeClientID,
	}

	// Also publish to Redis for other server instances
	h.publishToRedis(context.Background(), channelID, event)
}

// SendToUser sends an event to all connections of a specific user
func (h *Hub) SendToUser(userID uuid.UUID, event *Event) {
	h.mu.RLock()
	clients := h.userClients[userID]
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- data:
		default:
		}
	}
}

// SendUserEvent wraps SendToUser for callers that don't import the ws package.
func (h *Hub) SendUserEvent(userID uuid.UUID, eventType string, data any) {
	h.SendToUser(userID, &Event{Type: eventType, Data: data})
}

// SendToClient sends an event to a specific client
func (h *Hub) SendToClient(clientID uuid.UUID, event *Event) {
	h.mu.RLock()
	client, ok := h.clients[clientID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	select {
	case client.Send <- data:
	default:
	}
}

// GetOnlineUsers returns list of online users from a list of user IDs
func (h *Hub) GetOnlineUsers(userIDs []uuid.UUID) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	online := make([]uuid.UUID, 0)
	for _, userID := range userIDs {
		if _, ok := h.userClients[userID]; ok {
			online = append(online, userID)
		}
	}
	return online
}

// IsUserOnline checks if a user has any active connections
func (h *Hub) IsUserOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.userClients[userID]
	return ok
}

// GetUserConnectionCount returns the number of active connections for a user
func (h *Hub) GetUserConnectionCount(userID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.userClients[userID])
}

// Redis pub/sub for horizontal scaling
func (h *Hub) publishToRedis(ctx context.Context, channelID string, event *Event) {
	data := struct {
		ChannelID string `json:"channelId"`
		Event     *Event `json:"event"`
	}{
		ChannelID: channelID,
		Event:     event,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	h.redis.Publish(ctx, "websocket:broadcast", jsonData)
}

func (h *Hub) subscribeToRedis(ctx context.Context) {
	pubsub := h.redis.Subscribe(ctx, "websocket:broadcast")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			var data struct {
				ChannelID string `json:"channelId"`
				Event     *Event `json:"event"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &data); err != nil {
				continue
			}

			// Broadcast to local clients only (don't republish to Redis)
			h.broadcastToChannel(&BroadcastMessage{
				ChannelID: data.ChannelID,
				Event:     data.Event,
			})
		}
	}
}

// Presence management
func (h *Hub) setUserPresence(ctx context.Context, userID uuid.UUID, status string) {
	key := fmt.Sprintf("presence:%s", userID.String())
	h.redis.Set(ctx, key, status, 5*time.Minute)

	// Publish presence update event
	event := &Event{
		Type: EventTypePresenceUpdate,
		Data: map[string]interface{}{
			"userId": userID.String(),
			"status": status,
		},
	}

	// Broadcast to all channels the user is subscribed to
	// This would need user's community/channel list
	eventData, _ := json.Marshal(event)
	h.redis.Publish(ctx, fmt.Sprintf("presence:%s", userID.String()), eventData)
}

func (h *Hub) GetUserPresence(ctx context.Context, userID uuid.UUID) string {
	key := fmt.Sprintf("presence:%s", userID.String())
	status, err := h.redis.Get(ctx, key).Result()
	if err != nil {
		return "offline"
	}
	return status
}

// Typing indicators
func (h *Hub) SetTyping(ctx context.Context, channelID string, userID uuid.UUID) {
	key := fmt.Sprintf("typing:%s", channelID)
	h.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: userID.String(),
	})
	h.redis.Expire(ctx, key, 10*time.Second)

	// Fetch user info for the typing event
	u, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Str("userId", userID.String()).Msg("Failed to fetch user for typing event")
		return
	}

	// Broadcast typing event
	h.Broadcast(channelID, &Event{
		Type: EventTypeTypingStart,
		Data: map[string]interface{}{
			"channelId": channelID,
			"userId":    userID.String(),
			"user":      u,
		},
	}, nil)
}

func (h *Hub) GetTypingUsers(ctx context.Context, channelID string) []uuid.UUID {
	key := fmt.Sprintf("typing:%s", channelID)
	cutoff := float64(time.Now().Add(-5 * time.Second).Unix())

	members, err := h.redis.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", cutoff),
		Max: "+inf",
	}).Result()
	if err != nil {
		return nil
	}

	users := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		if id, err := uuid.Parse(m); err == nil {
			users = append(users, id)
		}
	}
	return users
}
