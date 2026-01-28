package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zentra/peridotite/pkg/database"
)

// RateLimitMiddleware limits requests per IP or user
func RateLimitMiddleware(redisClient *redis.Client, rps int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Use user ID if authenticated, otherwise use IP
			var key string
			if userID, ok := GetUserID(ctx); ok {
				key = fmt.Sprintf("user:%s", userID.String())
			} else {
				key = fmt.Sprintf("ip:%s", getClientIP(r))
			}

			// Rate limit window is 1 second
			count, err := database.IncrementRateLimit(ctx, key, time.Second)
			if err != nil {
				// If Redis fails, allow the request but log the error
				next.ServeHTTP(w, r)
				return
			}

			// Check if rate limit exceeded
			if count > int64(rps) {
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rps))
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "1")
				http.Error(w, `{"error":"Rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			remaining := int64(rps) - count
			if remaining < 0 {
				remaining = 0
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rps))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

			next.ServeHTTP(w, r)
		})
	}
}

// StrictRateLimitMiddleware applies stricter rate limiting for sensitive endpoints
func StrictRateLimitMiddleware(rps int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			key := fmt.Sprintf("strict:%s:%s", r.URL.Path, getClientIP(r))

			count, err := database.IncrementRateLimit(ctx, key, time.Minute)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if count > int64(rps) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"Too many requests, please try again later"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check common proxy headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}

// TimeoutMiddleware adds a timeout to request context
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					http.Error(w, `{"error":"Request timeout"}`, http.StatusGatewayTimeout)
				}
			}
		})
	}
}
