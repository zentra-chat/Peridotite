package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Environment string
	Server      struct {
		Port           string
		AllowedOrigins []string
		RateLimitRPS   int
		RateLimitBurst int
	}
	Database struct {
		URL string
	}
	Redis struct {
		URL string
	}
	Storage struct {
		Endpoint   string
		AccessKey  string
		SecretKey  string
		UseSSL     bool
		Bucket     string
		CDNBaseURL string
	}
	JWT struct {
		Secret     string
		AccessTTL  time.Duration
		RefreshTTL time.Duration
	}
	Encryption struct {
		Key string
	}
}

var AppConfig *Config

func Load() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found, using environment variables")
	}

	cfg := &Config{}

	cfg.Environment = getEnv("APP_ENV", "development")

	// Server
	cfg.Server.Port = getEnv("PORT", "8080")
	cfg.Server.AllowedOrigins = getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"})
	cfg.Server.RateLimitRPS = getEnvInt("RATE_LIMIT_RPS", 50)
	cfg.Server.RateLimitBurst = getEnvInt("RATE_LIMIT_BURST", 100)

	// Database
	postgresUser := getEnv("POSTGRES_USER", "zentra")
	postgresPass := getEnv("POSTGRES_PASSWORD", "zentra_secure_password")
	postgresHost := getEnv("POSTGRES_HOST", "localhost")
	postgresPort := getEnv("POSTGRES_PORT", "5432")
	postgresDB := getEnv("POSTGRES_DB", "zentra")
	postgresSSL := getEnv("POSTGRES_SSLMODE", "disable")
	cfg.Database.URL = getEnv("DATABASE_URL", "postgres://"+postgresUser+":"+postgresPass+"@"+postgresHost+":"+postgresPort+"/"+postgresDB+"?sslmode="+postgresSSL)

	// Redis
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	cfg.Redis.URL = getEnv("REDIS_URL", "redis://"+redisHost+":"+redisPort)

	// Storage
	cfg.Storage.Endpoint = getEnv("MINIO_ENDPOINT", "localhost:9000")
	cfg.Storage.AccessKey = getEnv("MINIO_ACCESS_KEY", "zentra_minio")
	cfg.Storage.SecretKey = getEnv("MINIO_SECRET_KEY", "zentra_minio_secret")
	cfg.Storage.UseSSL = getEnvBool("MINIO_USE_SSL", false)
	cfg.Storage.Bucket = getEnv("MINIO_BUCKET_ATTACHMENTS", "attachments")
	cfg.Storage.CDNBaseURL = getEnv("CDN_BASE_URL", "http://localhost:9000")

	// JWT
	cfg.JWT.Secret = getEnv("JWT_SECRET", "your-super-secret-jwt-key-change-in-production")
	cfg.JWT.AccessTTL = getEnvDuration("JWT_ACCESS_TOKEN_EXPIRY", 15*time.Minute)
	cfg.JWT.RefreshTTL = getEnvDuration("JWT_REFRESH_TOKEN_EXPIRY", 168*time.Hour)

	// Encryption
	cfg.Encryption.Key = getEnv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	AppConfig = cfg
	return cfg, nil
}

func (c *Config) GetPostgresURL() string {
	return c.Database.URL
}

func (c *Config) GetRedisAddr() string {
	return c.Redis.URL
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// When will I ever use this?
// Answer: Probably never, but here it is anyway.
func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		var result []string
		for _, v := range splitAndTrim(value, ",") {
			if v != "" {
				result = append(result, v)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

func splitAndTrim(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			part := trim(s[start:i])
			if part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		part := trim(s[start:])
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
