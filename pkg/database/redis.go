package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var RedisClient *redis.Client

func NewRedisClient(redisURL string) (*redis.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	RedisClient = client
	log.Info().Msg("Connected to Redis")

	return client, nil
}

func CloseRedis() {
	if RedisClient != nil {
		if err := RedisClient.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing Redis connection")
		} else {
			log.Info().Msg("Closed Redis connection")
		}
	}
}

// Redis key prefixes for organization
const (
	KeyPrefixSession      = "session:"
	KeyPrefixUserPresence = "presence:user:"
	KeyPrefixTyping       = "typing:"
	KeyPrefixRateLimit    = "ratelimit:"
	KeyPrefixOnlineUsers  = "online:"
	KeyPrefixMessageCache = "msgcache:"
)

// Session management
func SetSession(ctx context.Context, sessionID string, userID string, expiry time.Duration) error {
	return RedisClient.Set(ctx, KeyPrefixSession+sessionID, userID, expiry).Err()
}

func GetSession(ctx context.Context, sessionID string) (string, error) {
	return RedisClient.Get(ctx, KeyPrefixSession+sessionID).Result()
}

func DeleteSession(ctx context.Context, sessionID string) error {
	return RedisClient.Del(ctx, KeyPrefixSession+sessionID).Err()
}

// User presence
func SetUserPresence(ctx context.Context, userID string, status string, expiry time.Duration) error {
	return RedisClient.Set(ctx, KeyPrefixUserPresence+userID, status, expiry).Err()
}

func GetUserPresence(ctx context.Context, userID string) (string, error) {
	return RedisClient.Get(ctx, KeyPrefixUserPresence+userID).Result()
}

// Typing indicators
func SetTyping(ctx context.Context, channelID string, userID string) error {
	key := KeyPrefixTyping + channelID
	// Typing indicator expires after 10 seconds
	err := RedisClient.SAdd(ctx, key, userID).Err()
	if err != nil {
		return err
	}
	return RedisClient.Expire(ctx, key, 10*time.Second).Err()
}

func GetTypingUsers(ctx context.Context, channelID string) ([]string, error) {
	return RedisClient.SMembers(ctx, KeyPrefixTyping+channelID).Result()
}

func RemoveTyping(ctx context.Context, channelID string, userID string) error {
	return RedisClient.SRem(ctx, KeyPrefixTyping+channelID, userID).Err()
}

// Online users per community
func AddOnlineUser(ctx context.Context, communityID string, userID string) error {
	return RedisClient.SAdd(ctx, KeyPrefixOnlineUsers+communityID, userID).Err()
}

func RemoveOnlineUser(ctx context.Context, communityID string, userID string) error {
	return RedisClient.SRem(ctx, KeyPrefixOnlineUsers+communityID, userID).Err()
}

func GetOnlineUsers(ctx context.Context, communityID string) ([]string, error) {
	return RedisClient.SMembers(ctx, KeyPrefixOnlineUsers+communityID).Result()
}

func CountOnlineUsers(ctx context.Context, communityID string) (int64, error) {
	return RedisClient.SCard(ctx, KeyPrefixOnlineUsers+communityID).Result()
}

// Rate limiting
func IncrementRateLimit(ctx context.Context, key string, window time.Duration) (int64, error) {
	fullKey := KeyPrefixRateLimit + key
	pipe := RedisClient.Pipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func GetRateLimit(ctx context.Context, key string) (int64, error) {
	val, err := RedisClient.Get(ctx, KeyPrefixRateLimit+key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// Pub/Sub for real-time events
func Publish(ctx context.Context, channel string, message interface{}) error {
	return RedisClient.Publish(ctx, channel, message).Err()
}

func Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return RedisClient.Subscribe(ctx, channels...)
}
