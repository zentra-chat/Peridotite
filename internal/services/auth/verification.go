package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/zentra/peridotite/internal/models"
)

const (
	emailVerificationTokenPrefix = "auth:email_verify:token:"
	emailVerificationUserPrefix  = "auth:email_verify:user:"
)

type CaptchaConfig struct {
	Enabled   bool
	SecretKey string
	VerifyURL string
}

type EmailConfig struct {
	VerificationRequired bool
	SMTPHost             string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string
	FromAddress          string
	VerificationURL      string
	VerificationTokenTTL time.Duration
}

func (s *Service) captchaEnabled() bool {
	if !s.captchaConfig.Enabled {
		return false
	}

	return strings.TrimSpace(s.captchaConfig.SecretKey) != "" && strings.TrimSpace(s.captchaConfig.VerifyURL) != ""
}

func (s *Service) emailVerificationEnabled() bool {
	if !s.emailConfig.VerificationRequired {
		return false
	}

	return s.ensureEmailConfig() == nil
}

func (s *Service) validateCaptcha(ctx context.Context, token, clientIP string) error {
	if !s.captchaEnabled() {
		return nil
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return ErrCaptchaRequired
	}

	form := url.Values{}
	form.Set("secret", s.captchaConfig.SecretKey)
	form.Set("response", token)
	if ip := strings.TrimSpace(clientIP); ip != "" {
		form.Set("remoteip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.captchaConfig.VerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrCaptchaUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return ErrCaptchaUnavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ErrCaptchaUnavailable
	}

	var verifyResp struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return ErrCaptchaUnavailable
	}

	if !verifyResp.Success {
		return ErrCaptchaInvalid
	}

	return nil
}

func (s *Service) sendEmailVerification(ctx context.Context, user *models.User) error {
	if !s.emailVerificationEnabled() {
		return nil
	}

	if err := s.ensureEmailConfig(); err != nil {
		return err
	}

	token, err := s.issueEmailVerificationToken(ctx, user.ID)
	if err != nil {
		return err
	}

	verificationURL, err := s.buildVerificationURL(token, user.Email)
	if err != nil {
		return err
	}

	if err := s.deliverVerificationEmail(user.Email, user.Username, verificationURL); err != nil {
		return err
	}

	return nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidVerifyToken
	}

	userIDRaw, err := s.redis.Get(ctx, emailVerificationTokenPrefix+token).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrInvalidVerifyToken
		}
		return err
	}

	userID, err := uuid.Parse(userIDRaw)
	if err != nil {
		return ErrInvalidVerifyToken
	}

	result, err := s.db.Exec(ctx,
		`UPDATE users SET email_verified = TRUE, updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	pipe := s.redis.TxPipeline()
	pipe.Del(ctx, emailVerificationTokenPrefix+token)
	pipe.Del(ctx, emailVerificationUserPrefix+userID.String())
	_, _ = pipe.Exec(ctx)

	return nil
}

func (s *Service) ResendVerificationEmail(ctx context.Context, email string) error {
	if !s.emailVerificationEnabled() {
		return nil
	}

	normalizedEmail := strings.TrimSpace(strings.ToLower(email))
	if normalizedEmail == "" {
		return nil
	}

	user := &models.User{}
	err := s.db.QueryRow(ctx,
		`SELECT id, username, email, email_verified FROM users WHERE LOWER(email) = $1 AND deleted_at IS NULL`,
		normalizedEmail,
	).Scan(&user.ID, &user.Username, &user.Email, &user.EmailVerified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}

	if user.EmailVerified {
		return nil
	}

	return s.sendEmailVerification(ctx, user)
}

func (s *Service) ensureEmailConfig() error {
	if strings.TrimSpace(s.emailConfig.SMTPHost) == "" || strings.TrimSpace(s.emailConfig.FromAddress) == "" || strings.TrimSpace(s.emailConfig.VerificationURL) == "" {
		return ErrEmailNotConfigured
	}
	return nil
}

func (s *Service) issueEmailVerificationToken(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := generateToken(32)
	if err != nil {
		return "", err
	}

	ttl := s.emailConfig.VerificationTokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	userKey := emailVerificationUserPrefix + userID.String()
	oldToken, err := s.redis.Get(ctx, userKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", err
	}

	pipe := s.redis.TxPipeline()
	if oldToken != "" {
		pipe.Del(ctx, emailVerificationTokenPrefix+oldToken)
	}
	pipe.Set(ctx, emailVerificationTokenPrefix+token, userID.String(), ttl)
	pipe.Set(ctx, userKey, token, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Service) buildVerificationURL(token, email string) (string, error) {
	baseURL := strings.TrimSpace(s.emailConfig.VerificationURL)
	if baseURL == "" {
		return "", ErrEmailNotConfigured
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	query := parsedURL.Query()
	query.Set("token", token)
	query.Set("email", email)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}

func (s *Service) deliverVerificationEmail(toEmail, username, verificationURL string) error {
	fromAddress := strings.TrimSpace(s.emailConfig.FromAddress)
	parsedFrom, err := mail.ParseAddress(fromAddress)
	if err != nil {
		return ErrEmailNotConfigured
	}

	parsedTo, err := mail.ParseAddress(strings.TrimSpace(toEmail))
	if err != nil {
		return ErrEmailSendFailed
	}

	host := strings.TrimSpace(s.emailConfig.SMTPHost)
	port := s.emailConfig.SMTPPort
	if port <= 0 {
		port = 587
	}

	subject := "Verify your email for Zentra"
	expiry := s.emailConfig.VerificationTokenTTL
	if expiry <= 0 {
		expiry = 24 * time.Hour
	}

	greeting := username
	if strings.TrimSpace(greeting) == "" {
		greeting = "there"
	}

	plainBody := fmt.Sprintf("Hi %s,\r\n\r\nWelcome to Zentra. Verify the email address on this account by opening this link:\r\n%s\r\n\r\nThis link expires in %s.\r\n\r\nIf this was not requested, this message can be ignored.", greeting, verificationURL, expiry.String())
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", fromAddress, parsedTo.Address, subject, plainBody)

	var smtpAuth smtp.Auth
	if strings.TrimSpace(s.emailConfig.SMTPUsername) != "" || s.emailConfig.SMTPPassword != "" {
		smtpAuth = smtp.PlainAuth("", s.emailConfig.SMTPUsername, s.emailConfig.SMTPPassword, host)
	}

	if err := smtp.SendMail(fmt.Sprintf("%s:%d", host, port), smtpAuth, parsedFrom.Address, []string{parsedTo.Address}, []byte(message)); err != nil {
		return fmt.Errorf("%w: %v", ErrEmailSendFailed, err)
	}

	return nil
}

func generateToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
