package main

import (
	"context"
	"fmt"
	"log"
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

	"vector-db-service/handlers"
	"vector-db-service/services"
)

func main() {
	// Optimize for multi-core
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Load configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is required")
	}

	qdrantHost := os.Getenv("QDRANT_HOST")
	if qdrantHost == "" {
		log.Fatal("QDRANT_HOST environment variable is required")
	}

	qdrantPort := os.Getenv("QDRANT_PORT")
	if qdrantPort == "" {
		log.Fatal("QDRANT_PORT environment variable is required")
	}

	corsOrigins := os.Getenv("CORS_ALLOW_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "*"
	}

	corsMethods := os.Getenv("CORS_ALLOW_METHODS")
	if corsMethods == "" {
		corsMethods = "GET,POST,PUT,DELETE,OPTIONS"
	}

	corsHeaders := os.Getenv("CORS_ALLOW_HEADERS")
	if corsHeaders == "" {
		corsHeaders = "Origin, Content-Type, Accept"
	}

	qdrantService, err := services.NewQdrantService(qdrantHost, qdrantPort)
	if err != nil {
		log.Fatalf("Failed to connect to Qdrant: %v", err)
	}
	defer qdrantService.Close()

	app := fiber.New(fiber.Config{
		AppName:               "Vector DB Service",
		ServerHeader:          "Vector-DB",
		DisableStartupMessage: false,
		BodyLimit:             50 * 1024 * 1024, // 50MB
		Prefork:               false,            // Disabled for Docker
		ReadTimeout:           60 * time.Second,
		WriteTimeout:          60 * time.Second,
		IdleTimeout:           120 * time.Second,
		Concurrency:           256 * 1024,
		ReadBufferSize:        8192,
		WriteBufferSize:       8192,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))

	// Rate limiting
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins,
		AllowMethods: corsMethods,
		AllowHeaders: corsHeaders,
	}))

	handler := handlers.NewVectorDBHandler(qdrantService)

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "vector-db",
			"version": "1.0.0",
			"status":  "ok",
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "healthy",
			"service":     "vector-db",
			"qdrant_host": qdrantHost,
			"qdrant_port": qdrantPort,
		})
	})

	app.Post("/collections/ensure", handler.EnsureCollection)
	app.Post("/documents/add", handler.AddDocuments)
	app.Post("/documents/search", handler.SearchDocuments)
	app.Delete("/documents/delete/:bot_id", handler.DeleteDocuments)
	app.Get("/documents/stats/:bot_id", handler.GetStats)
	app.Get("/documents/list/:bot_id", handler.ListDocuments)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("🛑 Gracefully shutting down Vector DB Service...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("🚀 Vector DB Service starting on port %s (CPUs: %d)", port, runtime.NumCPU())
	log.Printf("📊 Connected to Qdrant at %s:%s", qdrantHost, qdrantPort)
	log.Printf("   CORS origins: %s", corsOrigins)
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Server stopped gracefully")
}
