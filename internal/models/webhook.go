package models

import (
	"time"

	"github.com/google/uuid"
)

type Webhook struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	ChannelID    uuid.UUID  `json:"channelId" db:"channel_id"`
	CommunityID  uuid.UUID  `json:"communityId" db:"community_id"`
	CreatedBy    uuid.UUID  `json:"createdBy" db:"created_by"`
	BotUserID    uuid.UUID  `json:"-" db:"bot_user_id"`
	Name         string     `json:"name" db:"name"`
	AvatarURL    *string    `json:"avatarUrl,omitempty" db:"avatar_url"`
	ProviderHint *string    `json:"providerHint,omitempty" db:"provider_hint"`
	TokenHash    string     `json:"-" db:"token_hash"`
	TokenPreview string     `json:"tokenPreview" db:"token_preview"`
	IsActive     bool       `json:"isActive" db:"is_active"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty" db:"last_used_at"`
	CreatedAt    time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt    time.Time  `json:"updatedAt" db:"updated_at"`
}
