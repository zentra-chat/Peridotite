package messaging

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/zentra/peridotite/pkg/encryption"
)

type ContentCipher interface {
	Encrypt(content string) (ciphertext []byte, nonce []byte, err error)
	Decrypt(ciphertext []byte, nonce []byte) (string, error)
}

type ChannelCipher struct {
	key []byte
}

type DMCipher struct {
	key []byte
}

func NewChannelCipher(key []byte) *ChannelCipher {
	return &ChannelCipher{key: key}
}

func NewDMCipher(key []byte) *DMCipher {
	return &DMCipher{key: key}
}

func (c *ChannelCipher) Encrypt(content string) ([]byte, []byte, error) {
	ciphertext, err := encryption.Encrypt([]byte(content), c.key)
	if err != nil {
		return nil, nil, err
	}
	return ciphertext, nil, nil
}

func (c *ChannelCipher) Decrypt(ciphertext []byte, _ []byte) (string, error) {
	plaintext, err := encryption.Decrypt(ciphertext, c.key)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (c *DMCipher) Encrypt(content string) ([]byte, []byte, error) {
	if len(c.key) != 32 {
		return nil, nil, encryption.ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(content), nil)
	return ciphertext, nonce, nil
}

func (c *DMCipher) Decrypt(ciphertext, nonce []byte) (string, error) {
	if len(c.key) != 32 {
		return "", encryption.ErrInvalidKeyLength
	}

	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", encryption.ErrDecryptionFailed
	}

	return string(plaintext), nil
}
