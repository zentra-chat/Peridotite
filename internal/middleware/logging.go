package middleware

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.size += n
	return n, err
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", requestID)

		ctx := r.Context()
		logger := log.With().Str("requestId", requestID).Logger()
		ctx = logger.WithContext(ctx)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggingMiddleware logs HTTP requests with timing and status
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		// Get logger from context or use default
		logger := zerolog.Ctx(r.Context())
		if logger.GetLevel() == zerolog.Disabled {
			logger = &log.Logger
		}

		event := logger.Info()
		if lrw.statusCode >= 400 && lrw.statusCode < 500 {
			event = logger.Warn()
		} else if lrw.statusCode >= 500 {
			event = logger.Error()
		}

		event.
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", lrw.statusCode).
			Int("size", lrw.size).
			Dur("duration", duration).
			Str("ip", getClientIP(r)).
			Str("userAgent", r.UserAgent()).
			Msg("HTTP request")
	})
}

// RecoveryMiddleware recovers from panics and returns 500
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger := zerolog.Ctx(r.Context())
				logger.Error().
					Interface("panic", err).
					Str("path", r.URL.Path).
					Msg("Recovered from panic")

				http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
