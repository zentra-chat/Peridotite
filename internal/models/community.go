package models

import (
	"time"

	"github.com/google/uuid"
)

type MemberRole string

const (
	MemberRoleOwner     MemberRole = "owner"
	MemberRoleAdmin     MemberRole = "admin"
	MemberRoleModerator MemberRole = "moderator"
	MemberRoleMember    MemberRole = "member"
)

type Community struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	Name        string     `json:"name" db:"name"`
	Description *string    `json:"description,omitempty" db:"description"`
	IconURL     *string    `json:"iconUrl,omitempty" db:"icon_url"`
	BannerURL   *string    `json:"bannerUrl,omitempty" db:"banner_url"`
	OwnerID     uuid.UUID  `json:"ownerId" db:"owner_id"`
	IsPublic    bool       `json:"isPublic" db:"is_public"`
	IsOpen      bool       `json:"isOpen" db:"is_open"`
	MemberCount int        `json:"memberCount" db:"member_count"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
	DeletedAt   *time.Time `json:"-" db:"deleted_at"`
}

type CommunityMember struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	CommunityID uuid.UUID  `json:"communityId" db:"community_id"`
	UserID      uuid.UUID  `json:"userId" db:"user_id"`
	Nickname    *string    `json:"nickname,omitempty" db:"nickname"`
	Role        MemberRole `json:"role" db:"role"`
	JoinedAt    time.Time  `json:"joinedAt" db:"joined_at"`
}

type CommunityMemberWithUser struct {
	CommunityMember
	User *PublicUser `json:"user,omitempty"`
}

type CommunityInvite struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	CommunityID uuid.UUID  `json:"communityId" db:"community_id"`
	Code        string     `json:"code" db:"code"`
	CreatedBy   uuid.UUID  `json:"createdBy" db:"created_by"`
	MaxUses     *int       `json:"maxUses,omitempty" db:"max_uses"`
	UseCount    int        `json:"useCount" db:"use_count"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty" db:"expires_at"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
}

type Role struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CommunityID uuid.UUID `json:"communityId" db:"community_id"`
	Name        string    `json:"name" db:"name"`
	Color       *string   `json:"color,omitempty" db:"color"`
	Position    int       `json:"position" db:"position"`
	Permissions int64     `json:"permissions" db:"permissions"`
	IsDefault   bool      `json:"isDefault" db:"is_default"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

type MemberRoleAssignment struct {
	MemberID uuid.UUID `json:"memberId" db:"member_id"`
	RoleID   uuid.UUID `json:"roleId" db:"role_id"`
}

// Permission flags (bitfield)
const (
	PermissionViewChannels    int64 = 1 << 0
	PermissionSendMessages    int64 = 1 << 1
	PermissionManageMessages  int64 = 1 << 2
	PermissionManageChannels  int64 = 1 << 3
	PermissionManageCommunity int64 = 1 << 4
	PermissionManageRoles     int64 = 1 << 5
	PermissionKickMembers     int64 = 1 << 6
	PermissionBanMembers      int64 = 1 << 7
	PermissionCreateInvites   int64 = 1 << 8
	PermissionAttachFiles     int64 = 1 << 9
	PermissionAddReactions    int64 = 1 << 10
	PermissionMentionEveryone int64 = 1 << 11
	PermissionPinMessages     int64 = 1 << 12
	PermissionManageWebhooks  int64 = 1 << 13
	PermissionViewAuditLog    int64 = 1 << 14
	PermissionAdministrator   int64 = 1 << 15

	// Combined permission sets
	PermissionAllText  int64 = PermissionViewChannels | PermissionSendMessages | PermissionAddReactions | PermissionAttachFiles | PermissionCreateInvites
	PermissionAllAdmin int64 = PermissionAdministrator | PermissionManageCommunity | PermissionManageChannels | PermissionManageRoles | PermissionManageMessages
)

func HasPermission(userPermissions, required int64) bool {
	// Administrators have all permissions
	if userPermissions&PermissionAdministrator != 0 {
		return true
	}
	return userPermissions&required == required
}
