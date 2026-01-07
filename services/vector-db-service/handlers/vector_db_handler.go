package handlers

import (
	"context"
	"log"
	"time"

	"vector-db-service/models"
	"vector-db-service/services"

	"github.com/gofiber/fiber/v2"
)

type VectorDBHandler struct {
	qdrant *services.QdrantService
}

func NewVectorDBHandler(qdrant *services.QdrantService) *VectorDBHandler {
	return &VectorDBHandler{
		qdrant: qdrant,
	}
}

func (h *VectorDBHandler) EnsureCollection(c *fiber.Ctx) error {
	var req models.EnsureCollectionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "Invalid request body",
		})
	}
	if req.BotID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "bot_id is required",
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.qdrant.EnsureCollection(ctx, req.BotID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	return c.JSON(models.Response{
		Success: true,
		Message: "Collection ensured",
	})
}

func (h *VectorDBHandler) AddDocuments(c *fiber.Ctx) error {
	var req models.AddDocumentsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "Invalid request body",
		})
	}
	if len(req.Texts) != len(req.Embeddings) || len(req.Texts) != len(req.Metadata) {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "texts, embeddings and metadata must have the same length",
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	docIDs, err := h.qdrant.AddDocuments(ctx, req.BotID, req.Texts, req.Embeddings, req.Metadata)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	return c.JSON(models.Response{
		Success: true,
		Message: "Documents added",
		Data: fiber.Map{
			"doc_ids": docIDs,
			"count":   len(docIDs),
		},
	})
}

func (h *VectorDBHandler) SearchDocuments(c *fiber.Ctx) error {
	var req models.SearchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "Invalid request body",
		})
	}

	// Debug logging
	log.Printf("[VectorDB Search] bot_id: %q, limit: %d, embedding_len: %d",
		req.BotID, req.Limit, len(req.QueryEmbedding))

	if len(req.QueryEmbedding) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "query_embedding is required",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use vector similarity search; fallback to full scan if empty
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	results, err := h.qdrant.SearchDocuments(ctx, req.BotID, req.QueryEmbedding, uint64(limit))
	if err != nil {
		log.Printf("[VectorDB Search] Error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	if len(results) == 0 {
		all, fallbackErr := h.qdrant.GetAllDocuments(ctx, req.BotID)
		if fallbackErr == nil {
			results = all
			log.Printf("[VectorDB Search] Fallback to full collection, got %d docs", len(results))
		}
	}
	log.Printf("[VectorDB Search] Found %d results for bot_id: %q (vector search)", len(results), req.BotID)
	return c.JSON(models.Response{
		Success: true,
		Data: fiber.Map{
			"documents": results,
			"count":     len(results),
		},
	})
}

func (h *VectorDBHandler) DeleteDocuments(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "bot_id is required",
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.qdrant.DeleteDocuments(ctx, botID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	return c.JSON(models.Response{
		Success: true,
		Message: "Documents deleted",
	})
}

func (h *VectorDBHandler) GetStats(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "bot_id is required",
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	count, err := h.qdrant.GetStats(ctx, botID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	return c.JSON(models.StatsResponse{
		Success:        true,
		BotID:          botID,
		TotalDocuments: count,
	})
}

func (h *VectorDBHandler) ListDocuments(c *fiber.Ctx) error {
	botID := c.Params("bot_id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.Response{
			Success: false,
			Error:   "bot_id is required",
		})
	}
	limit := c.QueryInt("limit", 10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	documents, err := h.qdrant.ListDocuments(ctx, botID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.Response{
			Success: false,
			Error:   err.Error(),
		})
	}
	return c.JSON(models.Response{
		Success: true,
		Data: fiber.Map{
			"documents": documents,
			"count":     len(documents),
		},
	})
}
