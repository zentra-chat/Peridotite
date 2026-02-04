package user

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/pkg/database"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrAlreadyBlocked = errors.New("user already blocked")
	ErrNotBlocked     = errors.New("user not blocked")
)

type Service struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewService(db *pgxpool.Pool, redis *redis.Client) *Service {
	return &Service{
		db:    db,
		redis: redis,
	}
}

func (s *Service) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, username, email, display_name, avatar_url, bio, status, custom_status,
		email_verified, two_factor_enabled, created_at, updated_at, last_seen_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.AvatarURL,
		&user.Bio, &user.Status, &user.CustomStatus, &user.EmailVerified,
		&user.TwoFactorEnabled, &user.CreatedAt, &user.UpdatedAt, &user.LastSeenAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (s *Service) GetPublicUser(ctx context.Context, id uuid.UUID) (*models.PublicUser, error) {
	user, err := s.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return user.ToPublic(), nil
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*models.PublicUser, error) {
	user := &models.User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, username, display_name, avatar_url, bio, status, custom_status, created_at
		FROM users WHERE username = $1 AND deleted_at IS NULL`,
		username,
	).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL,
		&user.Bio, &user.Status, &user.CustomStatus, &user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user.ToPublic(), nil
}

type UpdateProfileRequest struct {
	DisplayName  *string `json:"displayName" validate:"omitempty,max=64"`
	Bio          *string `json:"bio" validate:"omitempty,max=500"`
	CustomStatus *string `json:"customStatus" validate:"omitempty,max=128"`
}

func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateProfileRequest) (*models.User, error) {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET 
			display_name = COALESCE($2, display_name),
			bio = COALESCE($3, bio),
			custom_status = COALESCE($4, custom_status),
			updated_at = NOW()
		WHERE id = $1`,
		userID, req.DisplayName, req.Bio, req.CustomStatus,
	)
	if err != nil {
		return nil, err
	}

	return s.GetUserByID(ctx, userID)
}

func (s *Service) UpdateAvatar(ctx context.Context, userID uuid.UUID, avatarURL string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET avatar_url = $2, updated_at = NOW() WHERE id = $1`,
		userID, avatarURL,
	)
	return err
}

func (s *Service) RemoveAvatar(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET avatar_url = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (s *Service) UpdateStatus(ctx context.Context, userID uuid.UUID, status models.UserStatus) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET status = $2, updated_at = NOW() WHERE id = $1`,
		userID, status,
	)
	if err != nil {
		return err
	}

	// Also update Redis presence
	return database.SetUserPresence(ctx, userID.String(), string(status), 0)
}

func (s *Service) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*models.PublicUser, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var total int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users 
		WHERE deleted_at IS NULL AND (username ILIKE $1 OR display_name ILIKE $1)`,
		"%"+query+"%",
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx,
		`SELECT id, username, display_name, avatar_url, bio, status, custom_status, created_at
		FROM users 
		WHERE deleted_at IS NULL AND (username ILIKE $1 OR display_name ILIKE $1)
		ORDER BY username
		LIMIT $2 OFFSET $3`,
		"%"+query+"%", limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*models.PublicUser
	for rows.Next() {
		user := &models.PublicUser{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL,
			&user.Bio, &user.Status, &user.CustomStatus, &user.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}

	return users, total, nil
}

// User Settings

func (s *Service) GetSettings(ctx context.Context, userID uuid.UUID) (*models.UserSettings, error) {
	settings := &models.UserSettings{}
	err := s.db.QueryRow(ctx,
		`SELECT user_id, theme, notifications_enabled, sound_enabled, compact_mode, settings_json, updated_at
		FROM user_settings WHERE user_id = $1`,
		userID,
	).Scan(
		&settings.UserID, &settings.Theme, &settings.NotificationsEnabled,
		&settings.SoundEnabled, &settings.CompactMode, &settings.SettingsJSON, &settings.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Create default settings
			settings = &models.UserSettings{
				UserID:               userID,
				Theme:                "dark",
				NotificationsEnabled: true,
				SoundEnabled:         true,
				CompactMode:          false,
				SettingsJSON:         []byte("{}"),
			}
			_, err = s.db.Exec(ctx,
				`INSERT INTO user_settings (user_id, theme, notifications_enabled, sound_enabled, compact_mode, settings_json)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				settings.UserID, settings.Theme, settings.NotificationsEnabled,
				settings.SoundEnabled, settings.CompactMode, settings.SettingsJSON,
			)
			if err != nil {
				return nil, err
			}
			return settings, nil
		}
		return nil, err
	}
	return settings, nil
}

type UpdateSettingsRequest struct {
	Theme                *string `json:"theme" validate:"omitempty,oneof=dark light"`
	NotificationsEnabled *bool   `json:"notificationsEnabled"`
	SoundEnabled         *bool   `json:"soundEnabled"`
	CompactMode          *bool   `json:"compactMode"`
	SettingsJSON         []byte  `json:"settings"`
}

func (s *Service) UpdateSettings(ctx context.Context, userID uuid.UUID, req *UpdateSettingsRequest) (*models.UserSettings, error) {
	// Build dynamic update query
	query := `UPDATE user_settings SET updated_at = NOW()`
	args := []interface{}{userID}
	argNum := 2

	if req.Theme != nil {
		query += `, theme = $` + string(rune('0'+argNum))
		args = append(args, *req.Theme)
		argNum++
	}
	if req.NotificationsEnabled != nil {
		query += `, notifications_enabled = $` + string(rune('0'+argNum))
		args = append(args, *req.NotificationsEnabled)
		argNum++
	}
	if req.SoundEnabled != nil {
		query += `, sound_enabled = $` + string(rune('0'+argNum))
		args = append(args, *req.SoundEnabled)
		argNum++
	}
	if req.CompactMode != nil {
		query += `, compact_mode = $` + string(rune('0'+argNum))
		args = append(args, *req.CompactMode)
		argNum++
	}
	if req.SettingsJSON != nil {
		query += `, settings_json = $` + string(rune('0'+argNum))
		args = append(args, req.SettingsJSON)
		argNum++
	}

	query += ` WHERE user_id = $1`

	_, err := s.db.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return s.GetSettings(ctx, userID)
}

// Blocking

func (s *Service) BlockUser(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return errors.New("cannot block yourself")
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING`,
		blockerID, blockedID,
	)
	return err
}

func (s *Service) UnblockUser(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	result, err := s.db.Exec(ctx,
		`DELETE FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`,
		blockerID, blockedID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotBlocked
	}
	return nil
}

func (s *Service) GetBlockedUsers(ctx context.Context, userID uuid.UUID) ([]*models.PublicUser, error) {
	rows, err := s.db.Query(ctx,
		`SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.status, u.custom_status, u.created_at
		FROM user_blocks b
		JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.PublicUser
	for rows.Next() {
		user := &models.PublicUser{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.AvatarURL,
			&user.Bio, &user.Status, &user.CustomStatus, &user.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}

func (s *Service) IsBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2)`,
		blockerID, blockedID,
	).Scan(&exists)
	return exists, err
}
