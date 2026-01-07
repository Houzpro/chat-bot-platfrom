package main

import (
	"backend/auth"
	"backend/clients"
	"backend/config"
	"backend/database"
	"backend/handlers"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Optimize for multi-core CPU
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Connect to database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	db, err := database.NewDB(databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database connected")

	// Run auto migrations
	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("✓ Database migrations completed")

	// Initialize repositories
	userRepo := database.NewUserRepository(db)
	botRepo := database.NewBotRepository(db)

	// Initialize JWT service
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = auth.GenerateSecretKey()
		log.Printf("⚠️  Generated JWT_SECRET: %s (save this for production!)", jwtSecret)
	}
	jwtService := auth.NewJWTService(jwtSecret, 24*time.Hour) // 24h token expiration

	// Create HTTP client with connection pooling and optimized settings
	httpClient := &http.Client{
		Timeout: cfg.HTTPClient.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,
			IdleConnTimeout:     90 * time.Second,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// Initialize client and handlers
	serviceClient := clients.NewClient(httpClient)
	h := handlers.NewHandler(cfg, serviceClient)
	authHandler := handlers.NewAuthHandler(userRepo, jwtService)
	botHandler := handlers.NewBotHandler(botRepo)

	// Create Fiber app with optimizations for high load
	app := fiber.New(fiber.Config{
		AppName:                      "backend-gateway",
		Prefork:                      false,            // Disabled in Docker
		BodyLimit:                    50 * 1024 * 1024, // 50MB
		ReadTimeout:                  cfg.HTTPClient.Timeout,
		WriteTimeout:                 cfg.HTTPClient.Timeout,
		IdleTimeout:                  120 * time.Second,
		ReadBufferSize:               8192,
		WriteBufferSize:              8192,
		Concurrency:                  256 * 1024, // Max concurrent connections
		DisableKeepalive:             false,
		ReduceMemoryUsage:            false,
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
	})

	// Middleware
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))

	// Rate limiting for API protection
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		},
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: false,
	}))

	// Public routes (no authentication required)
	app.Get("/health", h.Health)
	app.Post("/api/v1/auth/register", authHandler.Register)
	app.Post("/api/v1/auth/login", authHandler.Login)
	app.Get("/api/v1/config/defaults", h.GetDefaults)

	// Public bot routes (for chat access)
	app.Get("/api/v1/bots/:id", botHandler.GetBot)
	app.Post("/api/v1/chat/public/:bot_id", h.PublicRAGChat) // Public chat endpoint

	// Protected routes (require authentication)
	protected := app.Group("/api/v1", auth.Middleware(jwtService))

	// Auth
	protected.Get("/auth/me", authHandler.Me)

	// Bot management (owner only)
	protected.Post("/bots", botHandler.CreateBot)
	protected.Get("/bots", botHandler.GetMyBots)
	protected.Put("/bots/:id", botHandler.UpdateBot)
	protected.Delete("/bots/:id", botHandler.DeleteBot)
	protected.Get("/bots/:id/documents", botHandler.GetBotDocuments)

	// Document upload (owner only)
	protected.Post("/bots/:id/documents/upload", h.UploadDocumentForBot)

	// RAG chat (owner or with bot_id)
	protected.Post("/chat/rag", h.RAGChat) // Legacy support

	// Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Gracefully shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	// Start server
	log.Printf("🚀 Backend gateway starting on port %s (CPUs: %d)", cfg.Server.Port, runtime.NumCPU())
	if err := app.Listen(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Server stopped gracefully")
}
