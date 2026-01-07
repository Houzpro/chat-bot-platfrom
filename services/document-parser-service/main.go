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

	"document-parser-service/handlers"
)

func main() {
	// Optimize for multi-core
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Load configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is required")
	}

	bodyLimit := os.Getenv("BODY_LIMIT")
	if bodyLimit == "" {
		bodyLimit = "52428800" // 50MB default
	}
	bodyLimitInt := 52428800
	fmt.Sscanf(bodyLimit, "%d", &bodyLimitInt)

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

	app := fiber.New(fiber.Config{
		AppName:                      "Document Parser Service",
		ServerHeader:                 "Document-Parser",
		DisableStartupMessage:        false,
		BodyLimit:                    bodyLimitInt,
		Prefork:                      false, // Disabled for Docker
		ReadTimeout:                  60 * time.Second,
		WriteTimeout:                 60 * time.Second,
		IdleTimeout:                  120 * time.Second,
		Concurrency:                  256 * 1024,
		ReadBufferSize:               16384,
		WriteBufferSize:              16384,
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: false,
	})

	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))

	// Rate limiting
	app.Use(limiter.New(limiter.Config{
		Max:        50,
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

	handler := handlers.NewDocumentHandler()

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "document-parser",
			"version": "1.0.0",
			"status":  "ok",
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "document-parser",
			"supported_formats": []string{
				".txt", ".pdf", ".docx", ".json", ".csv", ".xlsx", ".xls", ".html", ".htm", ".md",
			},
		})
	})

	app.Post("/parse", handler.ParseDocument)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("🛑 Gracefully shutting down Document Parser...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("🚀 Document Parser Service starting on port %s (CPUs: %d)", port, runtime.NumCPU())
	log.Printf("   Body limit: %d bytes", bodyLimitInt)
	log.Printf("   CORS origins: %s", corsOrigins)
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Server stopped gracefully")
}
