package main

import (
	"backend/auth"
	"backend/clients"
	"backend/config"
	"backend/database"
	"backend/handlers"
	"backend/services"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
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
	convRepo := database.NewConversationRepository(db)
	collabRepo := database.NewCollaboratorRepository(db)
	modelRepo := database.NewModelRepository(db)

	// Seed base models from /models/*.gguf so the BotForm dropdown has
	// something to show on first boot. Idempotent: files already registered
	// are reconciled to match GGUF_MODEL_FILE rather than duplicated.
	// MODELS_DIR defaults to /models (where docker-compose mounts the host
	// ./models volume), but operators can override it for local dev.
	modelsDir := os.Getenv("MODELS_DIR")
	if modelsDir == "" {
		modelsDir = "/models"
	}
	defaultLlamaEndpoint := os.Getenv("LLAMA_SERVER_URL")
	if defaultLlamaEndpoint == "" {
		defaultLlamaEndpoint = "http://llama-cpp:8080"
	}
	activeGGUF := os.Getenv("GGUF_MODEL_FILE")
	if err := database.SeedBaseModels(modelRepo, modelsDir, defaultLlamaEndpoint, activeGGUF); err != nil {
		log.Printf("⚠️  SeedBaseModels: %v", err)
	}

	// Container manager — wires the Docker daemon into the model registry.
	// Optional: if the docker socket isn't mounted (e.g. local `go run`
	// outside compose), we log and continue with `containerMgr == nil`,
	// which causes deploy/stop endpoints to return 503 instead of crashing.
	llamaImage := os.Getenv("LLAMA_IMAGE")
	llamaNetwork := os.Getenv("LLAMA_DOCKER_NETWORK")
	llamaModelsHostDir := os.Getenv("LLAMA_MODELS_HOST_DIR")
	if llamaModelsHostDir == "" {
		// Path on the docker HOST (not inside this container) where ./models
		// lives. The bind-mount in compose uses this path verbatim, so it
		// must point at the host filesystem, not /models inside backend.
		llamaModelsHostDir = "./models"
	}
	llamaPortMin, _ := strconv.Atoi(os.Getenv("LLAMA_PORT_MIN"))
	llamaPortMax, _ := strconv.Atoi(os.Getenv("LLAMA_PORT_MAX"))
	llamaCtx, _ := strconv.Atoi(os.Getenv("LLAMA_N_CTX"))
	llamaThreads, _ := strconv.Atoi(os.Getenv("LLAMA_N_THREADS"))
	llamaGPULayers, _ := strconv.Atoi(os.Getenv("LLAMA_N_GPU_LAYERS"))
	llamaParallel, _ := strconv.Atoi(os.Getenv("LLAMA_PARALLEL"))
	llamaUseGPU := os.Getenv("LLAMA_USE_GPU") == "true" || os.Getenv("LLAMA_USE_GPU") == "1"
	containerMgr, cmErr := services.NewContainerManager(modelRepo, services.ManagerConfig{
		Image:         llamaImage,
		Network:       llamaNetwork,
		ModelsHostDir: llamaModelsHostDir,
		PortMin:       llamaPortMin,
		PortMax:       llamaPortMax,
		NCtx:          llamaCtx,
		NThreads:      llamaThreads,
		NGPULayers:    llamaGPULayers,
		Parallel:      llamaParallel,
		UseGPU:        llamaUseGPU,
		CacheTypeK:    os.Getenv("LLAMA_CACHE_TYPE_K"),
		CacheTypeV:    os.Getenv("LLAMA_CACHE_TYPE_V"),
	})
	if cmErr != nil {
		log.Printf("⚠️  ContainerManager init failed: %v (deploy endpoints will be unavailable)", cmErr)
		containerMgr = nil
	} else {
		pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := containerMgr.Ping(pingCtx); err != nil {
			log.Printf("⚠️  Docker daemon unreachable: %v (deploy endpoints will be unavailable)", err)
			containerMgr = nil
		} else {
			log.Println("✓ Docker daemon reachable")
			// Reconcile DB rows with actual container state — clears stale
			// 'running' rows from a previous boot whose containers are gone.
			if existing, err := modelRepo.GetAll(); err == nil {
				containerMgr.Reconcile(pingCtx, existing)
			}
		}
		cancel()
	}

	// Bootstrap a system admin account on first start. Two modes:
	//   1) ADMIN_EMAIL only      → promote an existing registered user.
	//   2) ADMIN_EMAIL+PASSWORD  → create the user if missing (system account),
	//                               then promote. Useful for fresh installs so
	//                               the operator can log in immediately without
	//                               needing to register through the UI first.
	// Defaults (admin@local / change-me-admin) come from .env.example so a
	// `docker compose up` install lands you with a working admin login —
	// change them before deploying anywhere serious.
	if adminEmail := strings.TrimSpace(os.Getenv("ADMIN_EMAIL")); adminEmail != "" {
		adminEmail = strings.ToLower(adminEmail)
		adminPassword := os.Getenv("ADMIN_PASSWORD")
		user, err := userRepo.GetByEmail(adminEmail)
		if err != nil {
			if adminPassword == "" {
				log.Printf("⚠️  ADMIN_EMAIL=%s not found and no ADMIN_PASSWORD set — skipping bootstrap", adminEmail)
			} else {
				created, cerr := userRepo.Create(adminEmail, adminPassword, "Administrator")
				if cerr != nil {
					log.Printf("⚠️  Failed to create admin %s: %v", adminEmail, cerr)
				} else if serr := userRepo.SetRole(created.ID, "admin"); serr != nil {
					log.Printf("⚠️  Created admin %s but failed to set role: %v", adminEmail, serr)
				} else {
					log.Printf("✓ Created admin account %s", adminEmail)
				}
			}
		} else if user.Role != "admin" {
			if serr := userRepo.SetRole(user.ID, "admin"); serr != nil {
				log.Printf("⚠️  Failed to promote %s to admin: %v", adminEmail, serr)
			} else {
				log.Printf("✓ Promoted %s to admin", adminEmail)
			}
		}
	}

	// Initialize JWT service
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	jwtService := auth.NewJWTService(jwtSecret, 24*time.Hour)

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
	h := handlers.NewHandler(cfg, serviceClient, botRepo, convRepo, collabRepo, modelRepo)
	authHandler := handlers.NewAuthHandler(userRepo, jwtService)
	botHandler := handlers.NewBotHandler(botRepo, collabRepo, userRepo, modelRepo, cfg)
	convHandler := handlers.NewConversationHandler(convRepo, botRepo, collabRepo)
	analyticsHandler := handlers.NewAnalyticsHandler(convRepo, botRepo)
	adminHandler := handlers.NewAdminHandler(userRepo, botRepo, convRepo)
	modelHandler := handlers.NewModelHandler(modelRepo, containerMgr)

	// Create Fiber app with optimizations for high load
	app := fiber.New(fiber.Config{
		AppName:                      "backend-gateway",
		Prefork:                      false,            // Disabled in Docker
		BodyLimit:                    cfg.Upload.BodyLimit, // from env (BODY_LIMIT)
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
		AllowOrigins:     cfg.CORS.AllowOrigins,
		AllowMethods:     cfg.CORS.AllowMethods,
		AllowHeaders:     cfg.CORS.AllowHeaders,
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
	app.Post("/api/v1/public/messages/:message_id/feedback", convHandler.PublicSubmitFeedback)

	// Protected routes (require authentication)
	protected := app.Group("/api/v1", auth.Middleware(jwtService))

	// Auth
	protected.Get("/auth/me", authHandler.Me)

	// Bot management (owner only)
	protected.Post("/bots", botHandler.CreateBot)
	protected.Get("/bots", botHandler.GetMyBots)
	protected.Put("/bots/:id", botHandler.UpdateBot)
	protected.Delete("/bots/:id", h.DeleteBot)
	protected.Get("/bots/:id/documents", botHandler.GetBotDocuments)

	// Collaborators (owner manages, anyone can have a role)
	protected.Get("/bots/:id/collaborators", botHandler.ListCollaborators)
	protected.Post("/bots/:id/collaborators", botHandler.AddCollaborator)
	protected.Put("/bots/:id/collaborators/:user_id", botHandler.UpdateCollaborator)
	protected.Delete("/bots/:id/collaborators/:user_id", botHandler.RemoveCollaborator)

	// Document management (owner only)
	protected.Post("/bots/:id/documents/upload", h.UploadDocumentForBot)
	protected.Delete("/bots/:id/documents/:doc_id", h.DeleteBotDocument)

	// RAG chat
	protected.Post("/chat/rag", h.RAGChat)

	// Conversation management
	protected.Post("/conversations", convHandler.CreateConversation)
	protected.Get("/bots/:id/conversations", convHandler.GetBotConversations)
	protected.Get("/conversations/:conv_id", convHandler.GetConversation)
	protected.Delete("/conversations/:conv_id", convHandler.DeleteConversation)

	// Message feedback
	protected.Post("/messages/:message_id/feedback", convHandler.SubmitFeedback)
	protected.Get("/bots/:id/feedback/stats", convHandler.GetFeedbackStats)

	// Analytics
	protected.Get("/bots/:id/analytics", analyticsHandler.GetBotAnalytics)

	// Models registry — base models are world-readable, finetuned only to owner.
	// Deploy/stop drive Docker; they 503 if the daemon is unreachable.
	protected.Get("/models", modelHandler.ListModels)
	protected.Get("/models/:id", modelHandler.GetModel)
	protected.Post("/models/:id/deploy", modelHandler.DeployModel)
	protected.Post("/models/:id/stop", modelHandler.StopModel)
	protected.Delete("/models/:id", modelHandler.DeleteModel)

	// Admin (requires role='admin' on top of standard auth)
	admin := protected.Group("/admin", auth.AdminMiddleware(userRepo))
	admin.Get("/stats", adminHandler.GetStats)
	admin.Get("/users", adminHandler.ListUsers)
	admin.Put("/users/:id/role", adminHandler.SetUserRole)
	admin.Delete("/users/:id", adminHandler.DeleteUser)
	admin.Get("/bots", adminHandler.ListBots)
	admin.Delete("/bots/:id", adminHandler.DeleteBot)

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
