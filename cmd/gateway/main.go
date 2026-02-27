package main

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/zentra/peridotite/config"
	"github.com/zentra/peridotite/internal/middleware"
	"github.com/zentra/peridotite/internal/services/auth"
	"github.com/zentra/peridotite/internal/services/channel"
	"github.com/zentra/peridotite/internal/services/community"
	"github.com/zentra/peridotite/internal/services/dm"
	"github.com/zentra/peridotite/internal/services/media"
	"github.com/zentra/peridotite/internal/services/message"
	"github.com/zentra/peridotite/internal/services/notification"
	"github.com/zentra/peridotite/internal/services/user"
	"github.com/zentra/peridotite/internal/services/voice"
	"github.com/zentra/peridotite/internal/services/websocket"
	"github.com/zentra/peridotite/pkg/database"
	"github.com/zentra/peridotite/pkg/storage"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}
	log.Info().Bool("discordImportConfigured", cfg.Discord.ImportToken != "").Msg("Discord import configuration loaded")

	// Initialize logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Connect to PostgreSQL
	db, err := database.NewPostgresPool(cfg.Database.URL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}
	defer db.Close()
	log.Info().Msg("Connected to PostgreSQL")

	// Connect to Redis
	redisClient, err := database.NewRedisClient(cfg.Redis.URL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()
	log.Info().Msg("Connected to Redis")

	// Connect to MinIO
	minioClient, err := storage.ConnectMinIO(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to MinIO")
	}
	log.Info().Msg("Connected to MinIO")

	// Decode encryption key
	encKey, err := hex.DecodeString(cfg.Encryption.Key)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to decode encryption key (must be hex)")
	}

	// Initialize services
	authService := auth.NewService(db, redisClient, cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	userService := user.NewService(db, redisClient)
	communityService := community.NewService(db, redisClient, encKey)
	channelService := channel.NewService(db, communityService)
	messageService := message.NewService(db, redisClient, encKey, channelService)
	dmService := dm.NewService(db, redisClient, encKey, userService)
	mediaService := media.NewService(db, minioClient, [3]string{cfg.Storage.BucketAttachments, cfg.Storage.BucketAvatars, cfg.Storage.BucketCommunity}, cfg.Storage.CDNBaseURL, communityService)

	// Initialize voice service
	voiceService := voice.NewService(db, channelService, userService)

	// Initialize WebSocket hub
	wsHub := websocket.NewHub(redisClient, channelService, userService, dmService, voiceService)
	go wsHub.Run(context.Background())

	// Initialize notification service (depends on wsHub)
	notificationService := notification.NewService(db, wsHub)
	messageService.SetNotificationService(notificationService)
	dmService.SetNotificationService(notificationService)

	// Initialize handlers
	authHandler := auth.NewHandler(authService)
	userHandler := user.NewHandler(userService)
	communityHandler := community.NewHandler(communityService, cfg.Discord.ImportToken)
	channelHandler := channel.NewHandler(channelService)
	messageHandler := message.NewHandler(messageService)
	dmHandler := dm.NewHandler(dmService)
	mediaHandler := media.NewHandler(mediaService)
	wsHandler := websocket.NewHandler(wsHub, cfg.JWT.Secret)
	voiceHandler := voice.NewHandler(voiceService)
	notificationHandler := notification.NewHandler(notificationService)

	// Create router
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.LoggingMiddleware)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RedirectSlashes)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.Server.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "Origin"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
		Debug:            cfg.Environment == "development",
	}))

	// Security headers
	r.Use(middleware.SecurityHeadersMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(chimiddleware.Timeout(60 * time.Second))

		// Public routes
		r.Mount("/auth", authHandler.Routes())
		r.Mount("/communities", communityHandler.Routes(cfg.JWT.Secret))

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(cfg.JWT.Secret))

			// Rate limiting for authenticated users
			r.Use(middleware.RateLimitMiddleware(redisClient, cfg.Server.RateLimitRPS))

			r.Mount("/users", userHandler.Routes())
			r.Mount("/channels", channelHandler.Routes())
			r.Mount("/messages", messageHandler.Routes())
			r.Mount("/dms", dmHandler.Routes())
			r.Mount("/media", mediaHandler.Routes())
			r.Mount("/notifications", notificationHandler.Routes())
			r.Mount("/voice", voiceHandler.Routes())
		})
	})

	// WebSocket endpoint (separate from API versioning)
	r.Mount("/ws", wsHandler.Routes())

	// Create HTTP server
	server := &http.Server{
		Addr:    "0.0.0.0:" + cfg.Server.Port,
		Handler: r,
	}

	// Start server
	go func() {
		log.Info().Str("port", cfg.Server.Port).Msg("Starting API Gateway")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server stopped")
}
