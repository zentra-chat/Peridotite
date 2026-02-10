package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 4096
)

func NewClient(userID uuid.UUID, conn *websocket.Conn, hub *Hub) *Client {
	return &Client{
		ID:         uuid.New(),
		UserID:     userID,
		Conn:       conn,
		Send:       make(chan []byte, 256),
		Hub:        hub,
		Subscribed: make(map[string]bool),
		lastPing:   time.Now(),
	}
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		c.lastPing = time.Now()
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().
					Err(err).
					Str("clientId", c.ID.String()).
					Msg("WebSocket read error")
			}
			break
		}

		c.handleMessage(message)
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current WebSocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Client) handleMessage(message []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().
			Err(err).
			Str("clientId", c.ID.String()).
			Msg("Failed to parse client message")
		return
	}

	switch msg.Type {
	case "SUBSCRIBE":
		c.handleSubscribe(msg.Data)
	case "UNSUBSCRIBE":
		c.handleUnsubscribe(msg.Data)
	case "TYPING_START":
		c.handleTypingStart(msg.Data)
	case "HEARTBEAT":
		c.handleHeartbeat()
	case "PRESENCE_UPDATE":
		c.handlePresenceUpdate(msg.Data)
	default:
		log.Warn().
			Str("type", msg.Type).
			Str("clientId", c.ID.String()).
			Msg("Unknown message type")
	}
}

func (c *Client) handleSubscribe(data json.RawMessage) {
	var req struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	channelID, err := uuid.Parse(req.ChannelID)
	if err != nil {
		return
	}

	if !c.canAccessStream(context.Background(), channelID) {
		log.Warn().
			Str("channelId", req.ChannelID).
			Str("userId", c.UserID.String()).
			Msg("User attempted to subscribe to unauthorized channel")
		return
	}

	c.Hub.Subscribe(c, req.ChannelID)
}

func (c *Client) handleUnsubscribe(data json.RawMessage) {
	var req struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	c.Hub.Unsubscribe(c, req.ChannelID)
}

func (c *Client) handleTypingStart(data json.RawMessage) {
	var req struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	channelID, err := uuid.Parse(req.ChannelID)
	if err != nil {
		return
	}

	if !c.canAccessStream(context.Background(), channelID) {
		return
	}

	c.Hub.SetTyping(context.Background(), req.ChannelID, c.UserID)
}

func (c *Client) canAccessStream(ctx context.Context, channelID uuid.UUID) bool {
	if c.Hub.channelService != nil && c.Hub.channelService.CanAccessChannel(ctx, channelID, c.UserID) {
		return true
	}
	if c.Hub.dmService != nil && c.Hub.dmService.CanAccessConversation(ctx, channelID, c.UserID) {
		return true
	}
	return false
}

func (c *Client) handleHeartbeat() {
	event := &Event{
		Type: EventTypeHeartbeatAck,
		Data: map[string]interface{}{
			"timestamp": time.Now().UnixMilli(),
		},
	}

	data, _ := json.Marshal(event)
	select {
	case c.Send <- data:
	default:
	}
}

func (c *Client) handlePresenceUpdate(data json.RawMessage) {
	var req struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"online":  true,
		"idle":    true,
		"dnd":     true,
		"offline": true,
	}
	if !validStatuses[req.Status] {
		return
	}

	c.Hub.setUserPresence(context.Background(), c.UserID, req.Status)
}

// SendEvent sends an event directly to this client
func (c *Client) SendEvent(event *Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	select {
	case c.Send <- data:
	default:
	}
}
