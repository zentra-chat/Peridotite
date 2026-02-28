package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	CommunityID *uuid.UUID      `json:"communityId,omitempty" db:"community_id"`
	ActorID     uuid.UUID       `json:"actorId" db:"actor_id"`
	Action      string          `json:"action" db:"action"`
	TargetType  *string         `json:"targetType,omitempty" db:"target_type"`
	TargetID    *uuid.UUID      `json:"targetId,omitempty" db:"target_id"`
	Details     json.RawMessage `json:"details,omitempty" db:"details"`
	CreatedAt   time.Time       `json:"createdAt" db:"created_at"`
}

// Audit action types
const (
	AuditActionCommunityCreate = "community.create"
	AuditActionCommunityUpdate = "community.update"
	AuditActionCommunityDelete = "community.delete"
	AuditActionChannelCreate   = "channel.create"
	AuditActionChannelUpdate   = "channel.update"
	AuditActionChannelDelete   = "channel.delete"
	AuditActionMemberJoin      = "member.join"
	AuditActionMemberLeave     = "member.leave"
	AuditActionMemberKick      = "member.kick"
	AuditActionMemberBan       = "member.ban"
	AuditActionMemberUnban     = "member.unban"
	AuditActionRoleCreate      = "role.create"
	AuditActionRoleUpdate      = "role.update"
	AuditActionRoleDelete      = "role.delete"
	AuditActionInviteCreate    = "invite.create"
	AuditActionInviteDelete    = "invite.delete"
	AuditActionMessageDelete   = "message.delete"
	AuditActionMessagePin      = "message.pin"
	AuditActionMessageUnpin    = "message.unpin"
)

type AuditLogWithActor struct {
	AuditLog
	Actor *PublicUser `json:"actor,omitempty"`
}
