package community

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/pkg/auth"
	"github.com/zentra/peridotite/pkg/database"
)

var (
	ErrCommunityNotFound = errors.New("community not found")
	ErrNotMember         = errors.New("user is not a member of this community")
	ErrAlreadyMember     = errors.New("user is already a member of this community")
	ErrNotOwner          = errors.New("only the owner can perform this action")
	ErrInvalidInvite     = errors.New("invalid or expired invite")
	ErrInsufficientPerms = errors.New("insufficient permissions")
	ErrRoleNotFound      = errors.New("role not found")
	ErrCannotRemoveOwner = errors.New("cannot remove the owner")
)

type Service struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewService(db *pgxpool.Pool, redis *redis.Client) *Service {
	return &Service{db: db, redis: redis}
}

type CreateCommunityRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=1000"`
	IsPublic    bool    `json:"isPublic"`
	IsOpen      bool    `json:"isOpen"`
}

func (s *Service) broadcast(ctx context.Context, communityID uuid.UUID, eventType string, data interface{}) {
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
		ChannelID: "", // Global broadcast for now
		Event:     event,
	}

	jsonData, err := json.Marshal(broadcast)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal community update broadcast")
		return
	}

	err = s.redis.Publish(ctx, "websocket:broadcast", jsonData).Err()
	if err != nil {
		log.Error().Err(err).Msg("Failed to publish community update to Redis")
	}
}

func (s *Service) CreateCommunity(ctx context.Context, ownerID uuid.UUID, req *CreateCommunityRequest) (*models.Community, error) {
	community := &models.Community{
		ID:          uuid.New(),
		Name:        req.Name,
		Description: req.Description,
		OwnerID:     ownerID,
		IsPublic:    req.IsPublic,
		IsOpen:      req.IsOpen,
		MemberCount: 0, // Trigger on community_members will increment to 1
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := database.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Create community
		_, err := tx.Exec(ctx,
			`INSERT INTO communities (id, name, description, owner_id, is_public, is_open, member_count, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			community.ID, community.Name, community.Description, community.OwnerID,
			community.IsPublic, community.IsOpen, community.MemberCount, community.CreatedAt, community.UpdatedAt,
		)
		if err != nil {
			return err
		}

		// Add owner as member with owner role
		memberID := uuid.New()
		_, err = tx.Exec(ctx,
			`INSERT INTO community_members (id, community_id, user_id, role, joined_at)
			VALUES ($1, $2, $3, $4, NOW())`,
			memberID, community.ID, ownerID, models.MemberRoleOwner,
		)
		if err != nil {
			return err
		}

		// Create default role
		_, err = tx.Exec(ctx,
			`INSERT INTO roles (id, community_id, name, permissions, is_default, position)
			VALUES ($1, $2, 'Member', $3, TRUE, 0)`,
			uuid.New(), community.ID, models.PermissionAllText,
		)
		if err != nil {
			return err
		}

		// Create default general channel
		_, err = tx.Exec(ctx,
			`INSERT INTO channels (id, community_id, name, type, position)
			VALUES ($1, $2, 'general', 'text', 0)`,
			uuid.New(), community.ID,
		)
		if err != nil {
			return err
		}

		// Create audit log
		details, _ := json.Marshal(map[string]string{"name": community.Name})
		_, err = tx.Exec(ctx,
			`INSERT INTO audit_logs (id, community_id, actor_id, action, target_type, target_id, details)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			uuid.New(), community.ID, ownerID, models.AuditActionCommunityCreate, "community", community.ID, details,
		)
		return err
	})

	if err != nil {
		return nil, err
	}

	return community, nil
}

func (s *Service) GetCommunity(ctx context.Context, id uuid.UUID) (*models.Community, error) {
	community := &models.Community{}
	err := s.db.QueryRow(ctx,
		`SELECT id, name, description, icon_url, banner_url, owner_id, is_public, is_open, member_count, created_at, updated_at
		FROM communities WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&community.ID, &community.Name, &community.Description, &community.IconURL,
		&community.BannerURL, &community.OwnerID, &community.IsPublic, &community.IsOpen,
		&community.MemberCount, &community.CreatedAt, &community.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCommunityNotFound
		}
		return nil, err
	}
	return community, nil
}

func (s *Service) GetUserCommunities(ctx context.Context, userID uuid.UUID) ([]*models.Community, error) {
	rows, err := s.db.Query(ctx,
		`SELECT c.id, c.name, c.description, c.icon_url, c.banner_url, c.owner_id, 
		c.is_public, c.is_open, c.member_count, c.created_at, c.updated_at
		FROM communities c
		JOIN community_members cm ON cm.community_id = c.id
		WHERE cm.user_id = $1 AND c.deleted_at IS NULL
		ORDER BY c.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var communities []*models.Community
	for rows.Next() {
		c := &models.Community{}
		err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &c.IconURL, &c.BannerURL,
			&c.OwnerID, &c.IsPublic, &c.IsOpen, &c.MemberCount, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		communities = append(communities, c)
	}

	return communities, nil
}

func (s *Service) DiscoverCommunities(ctx context.Context, query string, limit, offset int) ([]*models.Community, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var total int64
	baseQuery := `WHERE is_public = TRUE AND deleted_at IS NULL`
	args := []interface{}{}

	if query != "" {
		baseQuery += ` AND (name ILIKE $1 OR description ILIKE $1)`
		args = append(args, "%"+query+"%")
	}

	countQuery := `SELECT COUNT(*) FROM communities ` + baseQuery
	err := s.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	selectQuery := `SELECT id, name, description, icon_url, banner_url, owner_id, is_public, is_open, member_count, created_at, updated_at
		FROM communities ` + baseQuery + ` ORDER BY member_count DESC LIMIT $` + string(rune('0'+len(args)+1)) + ` OFFSET $` + string(rune('0'+len(args)+2))
	args = append(args, limit, offset)

	rows, err := s.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var communities []*models.Community
	for rows.Next() {
		c := &models.Community{}
		err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &c.IconURL, &c.BannerURL,
			&c.OwnerID, &c.IsPublic, &c.IsOpen, &c.MemberCount, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		communities = append(communities, c)
	}

	return communities, total, nil
}

type UpdateCommunityRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=2,max=100"`
	Description *string `json:"description" validate:"omitempty,max=1000"`
	IsPublic    *bool   `json:"isPublic"`
	IsOpen      *bool   `json:"isOpen"`
}

func (s *Service) UpdateCommunity(ctx context.Context, communityID, userID uuid.UUID, req *UpdateCommunityRequest) (*models.Community, error) {
	// Check permissions
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity); err != nil {
		return nil, err
	}

	_, err := s.db.Exec(ctx,
		`UPDATE communities SET 
			name = COALESCE($2, name),
			description = COALESCE($3, description),
			is_public = COALESCE($4, is_public),
			is_open = COALESCE($5, is_open),
			updated_at = NOW()
		WHERE id = $1`,
		communityID, req.Name, req.Description, req.IsPublic, req.IsOpen,
	)
	if err != nil {
		return nil, err
	}

	community, err := s.GetCommunity(ctx, communityID)
	if err == nil {
		s.broadcast(ctx, communityID, "COMMUNITY_UPDATE", community)
	}

	return community, err
}

func (s *Service) UpdateCommunityIcon(ctx context.Context, communityID, userID uuid.UUID, iconURL string) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx,
		`UPDATE communities SET icon_url = $2, updated_at = NOW() WHERE id = $1`,
		communityID, iconURL,
	)
	if err == nil {
		if community, err := s.GetCommunity(ctx, communityID); err == nil {
			s.broadcast(ctx, communityID, "COMMUNITY_UPDATE", community)
		}
	}
	return err
}

func (s *Service) UpdateCommunityBanner(ctx context.Context, communityID, userID uuid.UUID, bannerURL string) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx,
		`UPDATE communities SET banner_url = $2, updated_at = NOW() WHERE id = $1`,
		communityID, bannerURL,
	)
	if err == nil {
		if community, err := s.GetCommunity(ctx, communityID); err == nil {
			s.broadcast(ctx, communityID, "COMMUNITY_UPDATE", community)
		}
	}
	return err
}

func (s *Service) RemoveCommunityIcon(ctx context.Context, communityID, userID uuid.UUID) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx,
		`UPDATE communities SET icon_url = NULL, updated_at = NOW() WHERE id = $1`,
		communityID,
	)
	if err == nil {
		if community, err := s.GetCommunity(ctx, communityID); err == nil {
			s.broadcast(ctx, communityID, "COMMUNITY_UPDATE", community)
		}
	}
	return err
}

func (s *Service) RemoveCommunityBanner(ctx context.Context, communityID, userID uuid.UUID) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity); err != nil {
		return err
	}

	_, err := s.db.Exec(ctx,
		`UPDATE communities SET banner_url = NULL, updated_at = NOW() WHERE id = $1`,
		communityID,
	)
	if err == nil {
		if community, err := s.GetCommunity(ctx, communityID); err == nil {
			s.broadcast(ctx, communityID, "COMMUNITY_UPDATE", community)
		}
	}
	return err
}

func (s *Service) DeleteCommunity(ctx context.Context, communityID, userID uuid.UUID) error {
	// Only owner can delete
	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}
	if community.OwnerID != userID {
		return ErrNotOwner
	}

	_, err = s.db.Exec(ctx,
		`UPDATE communities SET deleted_at = NOW() WHERE id = $1`,
		communityID,
	)
	return err
}

// Member Management

func (s *Service) GetMember(ctx context.Context, communityID, userID uuid.UUID) (*models.CommunityMember, error) {
	member := &models.CommunityMember{}
	err := s.db.QueryRow(ctx,
		`SELECT id, community_id, user_id, nickname, role, joined_at
		FROM community_members WHERE community_id = $1 AND user_id = $2`,
		communityID, userID,
	).Scan(&member.ID, &member.CommunityID, &member.UserID, &member.Nickname, &member.Role, &member.JoinedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotMember
		}
		return nil, err
	}
	return member, nil
}

func (s *Service) GetMembers(ctx context.Context, communityID uuid.UUID, limit, offset int) ([]*models.CommunityMemberWithUser, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var total int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM community_members WHERE community_id = $1`,
		communityID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT cm.id, cm.community_id, cm.user_id, cm.nickname, cm.role, cm.joined_at,
		u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
		FROM community_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.community_id = $1
		ORDER BY cm.role DESC, cm.joined_at
		LIMIT $2 OFFSET $3`,
		communityID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var members []*models.CommunityMemberWithUser
	for rows.Next() {
		m := &models.CommunityMemberWithUser{}
		u := &models.PublicUser{}
		err := rows.Scan(
			&m.ID, &m.CommunityID, &m.UserID, &m.Nickname, &m.Role, &m.JoinedAt,
			&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.Status, &u.CustomStatus, &u.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		m.User = u
		members = append(members, m)
	}

	return members, total, nil
}

func (s *Service) JoinCommunity(ctx context.Context, communityID, userID uuid.UUID) error {
	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}

	// Check if community is open for direct joins
	if !community.IsOpen && !community.IsPublic {
		return errors.New("this community requires an invite to join")
	}

	return s.addMember(ctx, communityID, userID)
}

func (s *Service) JoinWithInvite(ctx context.Context, code string, userID uuid.UUID) (*models.Community, error) {
	// Find and validate invite
	var invite models.CommunityInvite
	err := s.db.QueryRow(ctx,
		`SELECT id, community_id, max_uses, use_count, expires_at
		FROM community_invites WHERE code = $1`,
		code,
	).Scan(&invite.ID, &invite.CommunityID, &invite.MaxUses, &invite.UseCount, &invite.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidInvite
		}
		return nil, err
	}

	// Check expiration
	if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
		return nil, ErrInvalidInvite
	}

	// Check max uses
	if invite.MaxUses != nil && invite.UseCount >= *invite.MaxUses {
		return nil, ErrInvalidInvite
	}

	// Add member
	if err := s.addMember(ctx, invite.CommunityID, userID); err != nil {
		return nil, err
	}

	// Increment use count
	_, err = s.db.Exec(ctx,
		`UPDATE community_invites SET use_count = use_count + 1 WHERE id = $1`,
		invite.ID,
	)
	if err != nil {
		return nil, err
	}

	return s.GetCommunity(ctx, invite.CommunityID)
}

func (s *Service) addMember(ctx context.Context, communityID, userID uuid.UUID) error {
	// Check if already a member
	_, err := s.GetMember(ctx, communityID, userID)
	if err == nil {
		return ErrAlreadyMember
	}
	if err != ErrNotMember {
		return err
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO community_members (id, community_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4, NOW())`,
		uuid.New(), communityID, userID, models.MemberRoleMember,
	)
	return err
}

func (s *Service) LeaveCommunity(ctx context.Context, communityID, userID uuid.UUID) error {
	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}

	// Owner cannot leave (must transfer or delete)
	if community.OwnerID == userID {
		return errors.New("owner cannot leave the community, transfer ownership or delete it")
	}

	_, err = s.db.Exec(ctx,
		`DELETE FROM community_members WHERE community_id = $1 AND user_id = $2`,
		communityID, userID,
	)
	return err
}

func (s *Service) KickMember(ctx context.Context, communityID, actorID, targetID uuid.UUID) error {
	if err := s.requirePermission(ctx, communityID, actorID, models.PermissionKickMembers); err != nil {
		return err
	}

	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}
	if community.OwnerID == targetID {
		return ErrCannotRemoveOwner
	}

	_, err = s.db.Exec(ctx,
		`DELETE FROM community_members WHERE community_id = $1 AND user_id = $2`,
		communityID, targetID,
	)
	return err
}

func (s *Service) UpdateMemberRole(ctx context.Context, communityID, actorID, targetID uuid.UUID, role models.MemberRole) error {
	if err := s.requirePermission(ctx, communityID, actorID, models.PermissionManageRoles); err != nil {
		return err
	}

	// Cannot change owner role
	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}
	if community.OwnerID == targetID && role != models.MemberRoleOwner {
		return errors.New("cannot change owner's role")
	}

	_, err = s.db.Exec(ctx,
		`UPDATE community_members SET role = $3 WHERE community_id = $1 AND user_id = $2`,
		communityID, targetID, role,
	)
	return err
}

// Invites

func (s *Service) CreateInvite(ctx context.Context, communityID, userID uuid.UUID, maxUses *int, expiresIn *time.Duration) (*models.CommunityInvite, error) {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionCreateInvites); err != nil {
		return nil, err
	}

	code, err := auth.GenerateInviteCode()
	if err != nil {
		return nil, err
	}

	invite := &models.CommunityInvite{
		ID:          uuid.New(),
		CommunityID: communityID,
		Code:        code,
		CreatedBy:   userID,
		MaxUses:     maxUses,
		UseCount:    0,
		CreatedAt:   time.Now(),
	}

	if expiresIn != nil {
		expires := time.Now().Add(*expiresIn)
		invite.ExpiresAt = &expires
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO community_invites (id, community_id, code, created_by, max_uses, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		invite.ID, invite.CommunityID, invite.Code, invite.CreatedBy, invite.MaxUses, invite.ExpiresAt, invite.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return invite, nil
}

func (s *Service) GetInvites(ctx context.Context, communityID, userID uuid.UUID) ([]*models.CommunityInvite, error) {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionCreateInvites); err != nil {
		return nil, err
	}

	canManageAll := false
	if s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity) == nil ||
		s.requirePermission(ctx, communityID, userID, models.PermissionAdministrator) == nil {
		canManageAll = true
	}

	var rows pgx.Rows
	var err error
	if canManageAll {
		rows, err = s.db.Query(ctx,
			`SELECT id, community_id, code, created_by, max_uses, use_count, expires_at, created_at
			FROM community_invites WHERE community_id = $1
			ORDER BY created_at DESC`,
			communityID,
		)
	} else {
		rows, err = s.db.Query(ctx,
			`SELECT id, community_id, code, created_by, max_uses, use_count, expires_at, created_at
			FROM community_invites WHERE community_id = $1 AND created_by = $2
			ORDER BY created_at DESC`,
			communityID, userID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []*models.CommunityInvite
	for rows.Next() {
		i := &models.CommunityInvite{}
		err := rows.Scan(&i.ID, &i.CommunityID, &i.Code, &i.CreatedBy, &i.MaxUses, &i.UseCount, &i.ExpiresAt, &i.CreatedAt)
		if err != nil {
			return nil, err
		}
		invites = append(invites, i)
	}

	return invites, nil
}

func (s *Service) DeleteInvite(ctx context.Context, communityID, inviteID, userID uuid.UUID) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionCreateInvites); err != nil {
		return err
	}

	canManageAll := false
	if s.requirePermission(ctx, communityID, userID, models.PermissionManageCommunity) == nil ||
		s.requirePermission(ctx, communityID, userID, models.PermissionAdministrator) == nil {
		canManageAll = true
	}

	if canManageAll {
		_, err := s.db.Exec(ctx,
			`DELETE FROM community_invites WHERE id = $1 AND community_id = $2`,
			inviteID, communityID,
		)
		return err
	}

	_, err := s.db.Exec(ctx,
		`DELETE FROM community_invites WHERE id = $1 AND community_id = $2 AND created_by = $3`,
		inviteID, communityID, userID,
	)
	return err
}

// Roles

func (s *Service) GetRoles(ctx context.Context, communityID uuid.UUID) ([]*models.Role, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, community_id, name, color, position, permissions, is_default, created_at, updated_at
		FROM roles WHERE community_id = $1
		ORDER BY position DESC`,
		communityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*models.Role
	for rows.Next() {
		r := &models.Role{}
		err := rows.Scan(&r.ID, &r.CommunityID, &r.Name, &r.Color, &r.Position, &r.Permissions, &r.IsDefault, &r.CreatedAt, &r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}

	return roles, nil
}

type CreateRoleRequest struct {
	Name        string  `json:"name" validate:"required,min=1,max=64"`
	Color       *string `json:"color" validate:"omitempty,hexcolor"`
	Permissions int64   `json:"permissions"`
}

func (s *Service) CreateRole(ctx context.Context, communityID, userID uuid.UUID, req *CreateRoleRequest) (*models.Role, error) {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageRoles); err != nil {
		return nil, err
	}

	// Get max position
	var maxPos int
	s.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), 0) FROM roles WHERE community_id = $1`,
		communityID,
	).Scan(&maxPos)

	role := &models.Role{
		ID:          uuid.New(),
		CommunityID: communityID,
		Name:        req.Name,
		Color:       req.Color,
		Position:    maxPos + 1,
		Permissions: req.Permissions,
		IsDefault:   false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO roles (id, community_id, name, color, position, permissions, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		role.ID, role.CommunityID, role.Name, role.Color, role.Position, role.Permissions, role.IsDefault, role.CreatedAt, role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return role, nil
}

func (s *Service) DeleteRole(ctx context.Context, communityID, roleID, userID uuid.UUID) error {
	if err := s.requirePermission(ctx, communityID, userID, models.PermissionManageRoles); err != nil {
		return err
	}

	// Cannot delete default role
	var isDefault bool
	err := s.db.QueryRow(ctx,
		`SELECT is_default FROM roles WHERE id = $1 AND community_id = $2`,
		roleID, communityID,
	).Scan(&isDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRoleNotFound
		}
		return err
	}
	if isDefault {
		return errors.New("cannot delete the default role")
	}

	_, err = s.db.Exec(ctx, `DELETE FROM roles WHERE id = $1 AND community_id = $2`, roleID, communityID)
	return err
}

// Permission helpers

func (s *Service) RequirePermission(ctx context.Context, communityID, userID uuid.UUID, permission int64) error {
	return s.requirePermission(ctx, communityID, userID, permission)
}

func (s *Service) requirePermission(ctx context.Context, communityID, userID uuid.UUID, permission int64) error {
	member, err := s.GetMember(ctx, communityID, userID)
	if err != nil {
		return err
	}

	// Owner has all permissions
	community, err := s.GetCommunity(ctx, communityID)
	if err != nil {
		return err
	}
	if community.OwnerID == userID {
		return nil
	}

	// Admin role has all permissions
	if member.Role == models.MemberRoleAdmin || member.Role == models.MemberRoleOwner {
		return nil
	}

	// Check specific permissions via roles
	var userPermissions int64
	err = s.db.QueryRow(ctx,
		`SELECT COALESCE(BIT_OR(r.permissions), 0)
		FROM member_roles mr
		JOIN roles r ON r.id = mr.role_id
		WHERE mr.member_id = $1`,
		member.ID,
	).Scan(&userPermissions)
	if err != nil {
		// If no roles assigned, use default role permissions
		err = s.db.QueryRow(ctx,
			`SELECT permissions FROM roles WHERE community_id = $1 AND is_default = TRUE`,
			communityID,
		).Scan(&userPermissions)
		if err != nil {
			return ErrInsufficientPerms
		}
	}

	if !models.HasPermission(userPermissions, permission) {
		return ErrInsufficientPerms
	}

	return nil
}

func (s *Service) IsMember(ctx context.Context, communityID, userID uuid.UUID) bool {
	_, err := s.GetMember(ctx, communityID, userID)
	return err == nil
}
