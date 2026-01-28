package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	totpDigits = 6
	totpPeriod = 30
	totpSkew   = 1 // Allow 1 period before/after
)

// GenerateTOTPSecret creates a new TOTP secret for 2FA
func GenerateTOTPSecret() (string, error) {
	secret, err := GenerateSecureToken(20)
	if err != nil {
		return "", err
	}
	// Use base32 encoding for TOTP compatibility
	return base32.StdEncoding.EncodeToString([]byte(secret[:20])), nil
}

// GenerateTOTPURI creates the otpauth:// URI for QR codes
func GenerateTOTPURI(secret, username, issuer string) string {
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		issuer, username, secret, issuer, totpDigits, totpPeriod)
}

// ValidateTOTP validates a TOTP code against the secret
func ValidateTOTP(code string, secret string) bool {
	// Normalize input
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}

	// Decode the secret
	secretBytes, err := base32.StdEncoding.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}

	// Check current time and skew windows
	now := time.Now().Unix()
	counter := now / totpPeriod

	for i := -totpSkew; i <= totpSkew; i++ {
		if generateTOTP(secretBytes, counter+int64(i)) == code {
			return true
		}
	}

	return false
}

// generateTOTP generates a TOTP code for a given counter
func generateTOTP(secret []byte, counter int64) string {
	// Convert counter to bytes (big-endian)
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, uint64(counter))

	// Calculate HMAC-SHA1
	mac := hmac.New(sha1.New, secret)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	// Dynamic truncation
	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff

	// Generate code with leading zeros
	return fmt.Sprintf("%0*d", totpDigits, truncated%1000000)
}
