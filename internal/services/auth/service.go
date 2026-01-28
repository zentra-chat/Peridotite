package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/pkg/auth"
)

var (
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrInvalid2FA         = errors.New("invalid 2FA code")
	ErrUserNotFound       = errors.New("user not found")
)

type Service struct {
	db         *pgxpool.Pool
	redis      *redis.Client
	jwtSecret  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewService(db *pgxpool.Pool, redis *redis.Client, jwtSecret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{
		db:         db,
		redis:      redis,
		jwtSecret:  jwtSecret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

type RegisterRequest struct {
	Username string `json:"username" validate:"required,username"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,strongpassword"`
}

type LoginRequest struct {
	Login    string `json:"login" validate:"required"` // Username or email
	Password string `json:"password" validate:"required"`
	TOTPCode string `json:"totpCode,omitempty"`
}

type AuthResponse struct {
	User         *models.User `json:"user"`
	AccessToken  string       `json:"accessToken"`
	RefreshToken string       `json:"refreshToken"`
	ExpiresAt    time.Time    `json:"expiresAt"`
	Requires2FA  bool         `json:"requires2FA,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	// Check if username or email already exists
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 OR email = $2)`,
		req.Username, req.Email,
	).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// Create user
	user := &models.User{
		ID:           uuid.New(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Status:       models.UserStatusOnline,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO users (id, username, email, password_hash, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.Username, user.Email, user.PasswordHash, user.Status, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Create default user settings
	_, err = s.db.Exec(ctx,
		`INSERT INTO user_settings (user_id) VALUES ($1)`,
		user.ID,
	)
	if err != nil {
		return nil, err
	}

	// Generate tokens
	tokens, err := auth.GenerateTokenPair(user.ID, user.Username, s.jwtSecret, s.accessTTL)
	if err != nil {
		return nil, err
	}

	// Store refresh token session
	if err := s.createSession(ctx, user.ID, tokens.RefreshToken); err != nil {
		return nil, err
	}

	return &AuthResponse{
		User:         user,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
	}, nil
}

func (s *Service) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Find user by username or email
	user := &models.User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, username, email, password_hash, display_name, avatar_url, bio, 
		status, custom_status, email_verified, two_factor_enabled, two_factor_secret,
		created_at, updated_at, last_seen_at
		FROM users WHERE (username = $1 OR email = $1) AND deleted_at IS NULL`,
		req.Login,
	).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName,
		&user.AvatarURL, &user.Bio, &user.Status, &user.CustomStatus, &user.EmailVerified,
		&user.TwoFactorEnabled, &user.TwoFactorSecret, &user.CreatedAt, &user.UpdatedAt, &user.LastSeenAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Verify password
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check 2FA if enabled
	if user.TwoFactorEnabled {
		if req.TOTPCode == "" {
			return &AuthResponse{Requires2FA: true}, nil
		}
		if user.TwoFactorSecret == nil || !auth.ValidateTOTP(req.TOTPCode, *user.TwoFactorSecret) {
			return nil, ErrInvalid2FA
		}
	}

	// Generate tokens
	tokens, err := auth.GenerateTokenPair(user.ID, user.Username, s.jwtSecret, s.accessTTL)
	if err != nil {
		return nil, err
	}

	// Store refresh token session
	if err := s.createSession(ctx, user.ID, tokens.RefreshToken); err != nil {
		return nil, err
	}

	// Update last seen and status
	_, err = s.db.Exec(ctx,
		`UPDATE users SET last_seen_at = NOW(), status = 'online' WHERE id = $1`,
		user.ID,
	)
	if err != nil {
		return nil, err
	}

	user.Status = models.UserStatusOnline

	return &AuthResponse{
		User:         user,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
	}, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	tokenHash := auth.HashToken(refreshToken)

	// Find valid session
	var session models.UserSession
	var user models.User
	err := s.db.QueryRow(ctx,
		`SELECT s.id, s.user_id, s.expires_at, u.username
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.refresh_token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > NOW()`,
		tokenHash,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &user.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	// Revoke old session (token rotation)
	_, err = s.db.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = NOW() WHERE id = $1`,
		session.ID,
	)
	if err != nil {
		return nil, err
	}

	// Generate new tokens
	tokens, err := auth.GenerateTokenPair(session.UserID, user.Username, s.jwtSecret, s.accessTTL)
	if err != nil {
		return nil, err
	}

	// Create new session
	if err := s.createSession(ctx, session.UserID, tokens.RefreshToken); err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, userID uuid.UUID, refreshToken string) error {
	tokenHash := auth.HashToken(refreshToken)
	_, err := s.db.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = NOW() WHERE user_id = $1 AND refresh_token_hash = $2`,
		userID, tokenHash,
	)
	if err != nil {
		return err
	}

	// Update user status
	_, err = s.db.Exec(ctx,
		`UPDATE users SET status = 'offline', last_seen_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE user_sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

func (s *Service) createSession(ctx context.Context, userID uuid.UUID, refreshToken string) error {
	tokenHash := auth.HashToken(refreshToken)
	expiresAt := time.Now().Add(s.refreshTTL)

	_, err := s.db.Exec(ctx,
		`INSERT INTO user_sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

// 2FA Management

type Enable2FAResponse struct {
	Secret string `json:"secret"`
	OTPURI string `json:"otpUri"`
}

func (s *Service) Enable2FA(ctx context.Context, userID uuid.UUID) (*Enable2FAResponse, error) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}

	// Get username for URI
	var username string
	err = s.db.QueryRow(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	if err != nil {
		return nil, err
	}

	// Store secret temporarily (not enabled until verified)
	_, err = s.db.Exec(ctx,
		`UPDATE users SET two_factor_secret = $1 WHERE id = $2`,
		secret, userID,
	)
	if err != nil {
		return nil, err
	}

	return &Enable2FAResponse{
		Secret: secret,
		OTPURI: auth.GenerateTOTPURI(secret, username, "Zentra"),
	}, nil
}

func (s *Service) Verify2FA(ctx context.Context, userID uuid.UUID, code string) error {
	var secret *string
	err := s.db.QueryRow(ctx,
		`SELECT two_factor_secret FROM users WHERE id = $1`,
		userID,
	).Scan(&secret)
	if err != nil {
		return err
	}
	if secret == nil {
		return errors.New("2FA not set up")
	}

	if !auth.ValidateTOTP(code, *secret) {
		return ErrInvalid2FA
	}

	_, err = s.db.Exec(ctx,
		`UPDATE users SET two_factor_enabled = TRUE WHERE id = $1`,
		userID,
	)
	return err
}

func (s *Service) Disable2FA(ctx context.Context, userID uuid.UUID, password, code string) error {
	var passwordHash string
	var secret *string
	err := s.db.QueryRow(ctx,
		`SELECT password_hash, two_factor_secret FROM users WHERE id = $1`,
		userID,
	).Scan(&passwordHash, &secret)
	if err != nil {
		return err
	}

	// Verify password
	if !auth.VerifyPassword(password, passwordHash) {
		return ErrInvalidCredentials
	}

	// Verify current 2FA code
	if secret != nil && !auth.ValidateTOTP(code, *secret) {
		return ErrInvalid2FA
	}

	_, err = s.db.Exec(ctx,
		`UPDATE users SET two_factor_enabled = FALSE, two_factor_secret = NULL WHERE id = $1`,
		userID,
	)
	return err
}

// Password Management

func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	var passwordHash string
	err := s.db.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1`,
		userID,
	).Scan(&passwordHash)
	if err != nil {
		return err
	}

	if !auth.VerifyPassword(currentPassword, passwordHash) {
		return ErrInvalidCredentials
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID,
	)
	if err != nil {
		return err
	}

	// Revoke all sessions to force re-login
	return s.LogoutAll(ctx, userID)
}
