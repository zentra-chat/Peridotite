package dm

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/pkg/encryption"
)

var (
	ErrConversationNotFound = errors.New("conversation not found")
	ErrNotParticipant       = errors.New("not a conversation participant")
	ErrMessageNotFound      = errors.New("message not found")
	ErrNotMessageOwner      = errors.New("not message owner")
	ErrBlocked              = errors.New("user is blocked")
)

type Service struct {
	db            *pgxpool.Pool
	redis         *redis.Client
	encryptionKey []byte
	userService   UserServiceInterface
}

type UserServiceInterface interface {
	GetPublicUser(ctx context.Context, id uuid.UUID) (*models.PublicUser, error)
	IsBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error)
}

func NewService(db *pgxpool.Pool, redis *redis.Client, encryptionKey []byte, userService UserServiceInterface) *Service {
	return &Service{
		db:            db,
		redis:         redis,
		encryptionKey: encryptionKey,
		userService:   userService,
	}
}

type CreateConversationRequest struct {
	UserID uuid.UUID `json:"userId" validate:"required"`
}

type SendMessageRequest struct {
	Content string `json:"content" validate:"required,max=4000"`
}

type UpdateMessageRequest struct {
	Content string `json:"content" validate:"required,max=4000"`
}

type DMMessageResponse struct {
	ID             uuid.UUID          `json:"id"`
	ConversationID uuid.UUID          `json:"conversationId"`
	SenderID       uuid.UUID          `json:"senderId"`
	Content        string             `json:"content"`
	IsEdited       bool               `json:"isEdited"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	Sender         *models.PublicUser `json:"sender,omitempty"`
}

type DMConversationResponse struct {
	ID           uuid.UUID           `json:"id"`
	Participants []models.PublicUser `json:"participants"`
	LastMessage  *DMMessageResponse  `json:"lastMessage,omitempty"`
	UnreadCount  int                 `json:"unreadCount"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
}

type GetMessagesParams struct {
	Before *uuid.UUID
	After  *uuid.UUID
	Limit  int
}

func (s *Service) broadcast(ctx context.Context, conversationID string, eventType string, data interface{}) {
	event := struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}{
		Type: eventType,
		Data: data,
	}

	broadcast := struct {
		ChannelID string      `json:"channelId"`
		Event     interface{} `json:"event"`
	}{
		ChannelID: conversationID,
		Event:     event,
	}

	jsonData, err := json.Marshal(broadcast)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal DM broadcast")
		return
	}

	if err := s.redis.Publish(ctx, "websocket:broadcast", jsonData).Err(); err != nil {
		log.Error().Err(err).Msg("Failed to publish DM broadcast")
	}
}

func (s *Service) CanAccessConversation(ctx context.Context, conversationID, userID uuid.UUID) bool {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM dm_participants WHERE conversation_id = $1 AND user_id = $2)`,
		conversationID, userID,
	).Scan(&exists)
	return err == nil && exists
}

func (s *Service) CreateOrGetConversation(ctx context.Context, userID, otherUserID uuid.UUID) (*DMConversationResponse, error) {
	if userID == otherUserID {
		return nil, fmt.Errorf("cannot DM yourself")
	}

	if blocked, err := s.userService.IsBlocked(ctx, userID, otherUserID); err != nil {
		return nil, err
	} else if blocked {
		return nil, ErrBlocked
	}

	if blocked, err := s.userService.IsBlocked(ctx, otherUserID, userID); err != nil {
		return nil, err
	} else if blocked {
		return nil, ErrBlocked
	}

	var convo models.DMConversation
	err := s.db.QueryRow(ctx,
		`SELECT c.id, c.created_at, c.updated_at
		 FROM dm_conversations c
		 JOIN dm_participants p1 ON p1.conversation_id = c.id AND p1.user_id = $1
		 JOIN dm_participants p2 ON p2.conversation_id = c.id AND p2.user_id = $2
		 LIMIT 1`,
		userID, otherUserID,
	).Scan(&convo.ID, &convo.CreatedAt, &convo.UpdatedAt)
	if err == nil {
		return s.buildConversationResponse(ctx, convo, userID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	now := time.Now()
	convo = models.DMConversation{ID: uuid.New(), CreatedAt: now, UpdatedAt: now}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO dm_conversations (id, created_at, updated_at) VALUES ($1, $2, $2)`,
		convo.ID, now,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO dm_participants (conversation_id, user_id, last_read_at) VALUES ($1, $2, $3)`+
			`ON CONFLICT (conversation_id, user_id) DO NOTHING`,
		convo.ID, userID, now,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO dm_participants (conversation_id, user_id, last_read_at) VALUES ($1, $2, NULL)`+
			`ON CONFLICT (conversation_id, user_id) DO NOTHING`,
		convo.ID, otherUserID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.buildConversationResponse(ctx, convo, userID)
}

func (s *Service) ListConversations(ctx context.Context, userID uuid.UUID) ([]*DMConversationResponse, error) {
	rows, err := s.db.Query(ctx,
		`SELECT c.id, c.created_at, c.updated_at
		 FROM dm_conversations c
		 JOIN dm_participants p ON p.conversation_id = c.id
		 WHERE p.user_id = $1
		 ORDER BY c.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var responses []*DMConversationResponse
	for rows.Next() {
		var convo models.DMConversation
		if err := rows.Scan(&convo.ID, &convo.CreatedAt, &convo.UpdatedAt); err != nil {
			return nil, err
		}
		resp, err := s.buildConversationResponse(ctx, convo, userID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

func (s *Service) GetConversation(ctx context.Context, conversationID, userID uuid.UUID) (*DMConversationResponse, error) {
	if !s.CanAccessConversation(ctx, conversationID, userID) {
		return nil, ErrNotParticipant
	}

	var convo models.DMConversation
	err := s.db.QueryRow(ctx,
		`SELECT id, created_at, updated_at FROM dm_conversations WHERE id = $1`,
		conversationID,
	).Scan(&convo.ID, &convo.CreatedAt, &convo.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	return s.buildConversationResponse(ctx, convo, userID)
}

func (s *Service) GetMessages(ctx context.Context, conversationID, userID uuid.UUID, params *GetMessagesParams) ([]*DMMessageResponse, error) {
	if !s.CanAccessConversation(ctx, conversationID, userID) {
		return nil, ErrNotParticipant
	}

	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var query string
	var args []interface{}

	if params.Before != nil {
		query = `
			SELECT m.id, m.conversation_id, m.sender_id, m.encrypted_content, m.nonce, m.is_edited, m.created_at, m.updated_at,
			       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
			FROM direct_messages m
			JOIN users u ON u.id = m.sender_id
			WHERE m.conversation_id = $1 AND m.deleted_at IS NULL
			  AND m.created_at < (SELECT created_at FROM direct_messages WHERE id = $2)
			ORDER BY m.created_at DESC
			LIMIT $3`
		args = []interface{}{conversationID, *params.Before, limit}
	} else if params.After != nil {
		query = `
			SELECT m.id, m.conversation_id, m.sender_id, m.encrypted_content, m.nonce, m.is_edited, m.created_at, m.updated_at,
			       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
			FROM direct_messages m
			JOIN users u ON u.id = m.sender_id
			WHERE m.conversation_id = $1 AND m.deleted_at IS NULL
			  AND m.created_at > (SELECT created_at FROM direct_messages WHERE id = $2)
			ORDER BY m.created_at ASC
			LIMIT $3`
		args = []interface{}{conversationID, *params.After, limit}
	} else {
		query = `
			SELECT m.id, m.conversation_id, m.sender_id, m.encrypted_content, m.nonce, m.is_edited, m.created_at, m.updated_at,
			       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
			FROM direct_messages m
			JOIN users u ON u.id = m.sender_id
			WHERE m.conversation_id = $1 AND m.deleted_at IS NULL
			ORDER BY m.created_at DESC
			LIMIT $2`
		args = []interface{}{conversationID, limit}
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*DMMessageResponse
	for rows.Next() {
		var msg models.DirectMessage
		var nonce []byte
		var sender models.PublicUser

		if err := rows.Scan(
			&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.EncryptedContent, &nonce,
			&msg.IsEdited, &msg.CreatedAt, &msg.UpdatedAt,
			&sender.ID, &sender.Username, &sender.DisplayName, &sender.AvatarURL, &sender.Bio, &sender.Status, &sender.CustomStatus, &sender.CreatedAt,
		); err != nil {
			return nil, err
		}

		content, err := s.decryptContent(msg.EncryptedContent, nonce)
		if err != nil {
			content = "[Decryption Error]"
		}

		messages = append(messages, &DMMessageResponse{
			ID:             msg.ID,
			ConversationID: msg.ConversationID,
			SenderID:       msg.SenderID,
			Content:        content,
			IsEdited:       msg.IsEdited,
			CreatedAt:      msg.CreatedAt,
			UpdatedAt:      msg.UpdatedAt,
			Sender:         &sender,
		})
	}

	return messages, nil
}

func (s *Service) SendMessage(ctx context.Context, conversationID, userID uuid.UUID, req *SendMessageRequest) (*DMMessageResponse, error) {
	if !s.CanAccessConversation(ctx, conversationID, userID) {
		return nil, ErrNotParticipant
	}

	ciphertext, nonce, err := s.encryptContent(req.Content)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	messageID := uuid.New()

	_, err = s.db.Exec(ctx,
		`INSERT INTO direct_messages (id, conversation_id, sender_id, encrypted_content, nonce, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		messageID, conversationID, userID, ciphertext, nonce, now,
	)
	if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE dm_conversations SET updated_at = $2 WHERE id = $1`,
		conversationID, now,
	)
	if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE dm_participants SET last_read_at = $3 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID, now,
	)
	if err != nil {
		return nil, err
	}

	resp, err := s.GetMessage(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}

	s.broadcast(ctx, conversationID.String(), "DM_MESSAGE_CREATE", resp)

	return resp, nil
}

func (s *Service) GetMessage(ctx context.Context, messageID, userID uuid.UUID) (*DMMessageResponse, error) {
	var msg models.DirectMessage
	var nonce []byte
	var sender models.PublicUser

	err := s.db.QueryRow(ctx,
		`SELECT m.id, m.conversation_id, m.sender_id, m.encrypted_content, m.nonce, m.is_edited, m.created_at, m.updated_at,
		        u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
		 FROM direct_messages m
		 JOIN users u ON u.id = m.sender_id
		 WHERE m.id = $1 AND m.deleted_at IS NULL`,
		messageID,
	).Scan(
		&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.EncryptedContent, &nonce, &msg.IsEdited, &msg.CreatedAt, &msg.UpdatedAt,
		&sender.ID, &sender.Username, &sender.DisplayName, &sender.AvatarURL, &sender.Bio, &sender.Status, &sender.CustomStatus, &sender.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}

	if !s.CanAccessConversation(ctx, msg.ConversationID, userID) {
		return nil, ErrNotParticipant
	}

	content, err := s.decryptContent(msg.EncryptedContent, nonce)
	if err != nil {
		content = "[Decryption Error]"
	}

	return &DMMessageResponse{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        content,
		IsEdited:       msg.IsEdited,
		CreatedAt:      msg.CreatedAt,
		UpdatedAt:      msg.UpdatedAt,
		Sender:         &sender,
	}, nil
}

func (s *Service) UpdateMessage(ctx context.Context, messageID, userID uuid.UUID, req *UpdateMessageRequest) (*DMMessageResponse, error) {
	var senderID uuid.UUID
	var conversationID uuid.UUID

	err := s.db.QueryRow(ctx,
		`SELECT sender_id, conversation_id FROM direct_messages WHERE id = $1 AND deleted_at IS NULL`,
		messageID,
	).Scan(&senderID, &conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}

	if senderID != userID {
		return nil, ErrNotMessageOwner
	}

	ciphertext, nonce, err := s.encryptContent(req.Content)
	if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE direct_messages SET encrypted_content = $1, nonce = $2, is_edited = TRUE, updated_at = $3 WHERE id = $4`,
		ciphertext, nonce, time.Now(), messageID,
	)
	if err != nil {
		return nil, err
	}

	resp, err := s.GetMessage(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}

	s.broadcast(ctx, conversationID.String(), "DM_MESSAGE_UPDATE", resp)

	return resp, nil
}

func (s *Service) DeleteMessage(ctx context.Context, messageID, userID uuid.UUID) error {
	var senderID uuid.UUID
	var conversationID uuid.UUID

	err := s.db.QueryRow(ctx,
		`SELECT sender_id, conversation_id FROM direct_messages WHERE id = $1 AND deleted_at IS NULL`,
		messageID,
	).Scan(&senderID, &conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMessageNotFound
		}
		return err
	}

	if senderID != userID {
		return ErrNotMessageOwner
	}

	_, err = s.db.Exec(ctx,
		`UPDATE direct_messages SET deleted_at = $1 WHERE id = $2`,
		time.Now(), messageID,
	)
	if err != nil {
		return err
	}

	s.broadcast(ctx, conversationID.String(), "DM_MESSAGE_DELETE", map[string]interface{}{
		"conversationId": conversationID.String(),
		"messageId":      messageID.String(),
	})

	return nil
}

func (s *Service) MarkRead(ctx context.Context, conversationID, userID uuid.UUID) error {
	if !s.CanAccessConversation(ctx, conversationID, userID) {
		return ErrNotParticipant
	}

	_, err := s.db.Exec(ctx,
		`UPDATE dm_participants SET last_read_at = $3 WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID, time.Now(),
	)
	return err
}

func (s *Service) buildConversationResponse(ctx context.Context, convo models.DMConversation, userID uuid.UUID) (*DMConversationResponse, error) {
	participants, err := s.getParticipants(ctx, convo.ID)
	if err != nil {
		return nil, err
	}

	lastMessage, err := s.getLastMessage(ctx, convo.ID, participants)
	if err != nil {
		return nil, err
	}

	unreadCount, err := s.getUnreadCount(ctx, convo.ID, userID)
	if err != nil {
		return nil, err
	}

	return &DMConversationResponse{
		ID:           convo.ID,
		Participants: participants,
		LastMessage:  lastMessage,
		UnreadCount:  unreadCount,
		CreatedAt:    convo.CreatedAt,
		UpdatedAt:    convo.UpdatedAt,
	}, nil
}

func (s *Service) getParticipants(ctx context.Context, conversationID uuid.UUID) ([]models.PublicUser, error) {
	rows, err := s.db.Query(ctx,
		`SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
		 FROM dm_participants p
		 JOIN users u ON u.id = p.user_id
		 WHERE p.conversation_id = $1 AND u.deleted_at IS NULL`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []models.PublicUser
	for rows.Next() {
		var user models.PublicUser
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Status, &user.CustomStatus, &user.CreatedAt); err != nil {
			return nil, err
		}
		participants = append(participants, user)
	}

	return participants, nil
}

func (s *Service) getLastMessage(ctx context.Context, conversationID uuid.UUID, participants []models.PublicUser) (*DMMessageResponse, error) {
	var msg models.DirectMessage
	var nonce []byte

	err := s.db.QueryRow(ctx,
		`SELECT id, conversation_id, sender_id, encrypted_content, nonce, is_edited, created_at, updated_at
		 FROM direct_messages
		 WHERE conversation_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT 1`,
		conversationID,
	).Scan(&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.EncryptedContent, &nonce, &msg.IsEdited, &msg.CreatedAt, &msg.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	content, err := s.decryptContent(msg.EncryptedContent, nonce)
	if err != nil {
		content = "[Decryption Error]"
	}

	var sender *models.PublicUser
	for i := range participants {
		if participants[i].ID == msg.SenderID {
			sender = &participants[i]
			break
		}
	}
	if sender == nil {
		u, err := s.userService.GetPublicUser(ctx, msg.SenderID)
		if err == nil {
			sender = u
		}
	}

	return &DMMessageResponse{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		SenderID:       msg.SenderID,
		Content:        content,
		IsEdited:       msg.IsEdited,
		CreatedAt:      msg.CreatedAt,
		UpdatedAt:      msg.UpdatedAt,
		Sender:         sender,
	}, nil
}

func (s *Service) getUnreadCount(ctx context.Context, conversationID, userID uuid.UUID) (int, error) {
	var count int
	var lastRead *time.Time

	_ = s.db.QueryRow(ctx,
		`SELECT last_read_at FROM dm_participants WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&lastRead)

	if lastRead == nil {
		lastReadTime := time.Unix(0, 0)
		lastRead = &lastReadTime
	}

	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM direct_messages
		 WHERE conversation_id = $1 AND deleted_at IS NULL
		   AND created_at > $2 AND sender_id <> $3`,
		conversationID, *lastRead, userID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Service) encryptContent(content string) ([]byte, []byte, error) {
	if len(s.encryptionKey) != 32 {
		return nil, nil, encryption.ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(content), nil)
	return ciphertext, nonce, nil
}

func (s *Service) decryptContent(ciphertext, nonce []byte) (string, error) {
	if len(s.encryptionKey) != 32 {
		return "", encryption.ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", encryption.ErrDecryptionFailed
	}

	return string(plaintext), nil
}
