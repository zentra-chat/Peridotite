package models

import (
	"time"

	"github.com/google/uuid"
)

type CustomEmoji struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CommunityID uuid.UUID `json:"communityId" db:"community_id"`
	Name        string    `json:"name" db:"name"`
	ImageURL    string    `json:"imageUrl" db:"image_url"`
	UploaderID  uuid.UUID `json:"uploaderId" db:"uploader_id"`
	Animated    bool      `json:"animated" db:"animated"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

// CustomEmojiWithCommunity includes the community name for cross-server usage
type CustomEmojiWithCommunity struct {
	CustomEmoji
	CommunityName string `json:"communityName" db:"community_name"`
}
