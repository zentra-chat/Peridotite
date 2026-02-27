package models

import (
	"time"

	"github.com/google/uuid"
)

type ChannelType string

const (
	ChannelTypeText         ChannelType = "text"
	ChannelTypeAnnouncement ChannelType = "announcement"
	ChannelTypeGallery      ChannelType = "gallery"
	ChannelTypeForum        ChannelType = "forum"
	ChannelTypeVoice        ChannelType = "voice"
)

// VoiceState represents a user's voice connection state in a voice channel
type VoiceState struct {
	ID          uuid.UUID `json:"id" db:"id"`
	ChannelID   uuid.UUID `json:"channelId" db:"channel_id"`
	UserID      uuid.UUID `json:"userId" db:"user_id"`
	IsMuted     bool      `json:"isMuted" db:"is_muted"`
	IsDeafened  bool      `json:"isDeafened" db:"is_deafened"`
	IsSelfMuted bool      `json:"isSelfMuted" db:"is_self_muted"`
	IsSelfDeaf  bool      `json:"isSelfDeafened" db:"is_self_deafened"`
	JoinedAt    time.Time `json:"joinedAt" db:"joined_at"`
}

// VoiceStateWithUser includes user info for display
type VoiceStateWithUser struct {
	VoiceState
	User *User `json:"user,omitempty"`
}

type ChannelCategory struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CommunityID uuid.UUID `json:"communityId" db:"community_id"`
	Name        string    `json:"name" db:"name"`
	Position    int       `json:"position" db:"position"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}

type Channel struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	CommunityID     uuid.UUID   `json:"communityId" db:"community_id"`
	CategoryID      *uuid.UUID  `json:"categoryId,omitempty" db:"category_id"`
	Name            string      `json:"name" db:"name"`
	Topic           *string     `json:"topic,omitempty" db:"topic"`
	Type            ChannelType `json:"type" db:"type"`
	Position        int         `json:"position" db:"position"`
	IsNSFW          bool        `json:"isNsfw" db:"is_nsfw"`
	SlowmodeSeconds int         `json:"slowmodeSeconds" db:"slowmode_seconds"`
	LastMessageAt   *time.Time  `json:"lastMessageAt,omitempty" db:"last_message_at"`
	CreatedAt       time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time   `json:"updatedAt" db:"updated_at"`
}

type ChannelPermission struct {
	ID               uuid.UUID `json:"id" db:"id"`
	ChannelID        uuid.UUID `json:"channelId" db:"channel_id"`
	TargetType       string    `json:"targetType" db:"target_type"` // "role" or "member"
	TargetID         uuid.UUID `json:"targetId" db:"target_id"`
	AllowPermissions int64     `json:"allowPermissions" db:"allow_permissions"`
	DenyPermissions  int64     `json:"denyPermissions" db:"deny_permissions"`
}

type ChannelWithCategory struct {
	Channel
	CategoryName *string `json:"categoryName,omitempty" db:"category_name"`
}
