package handlers

import (
	"backend/auth"
	"backend/database"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type BotHandler struct {
	botRepo *database.BotRepository
}

func NewBotHandler(botRepo *database.BotRepository) *BotHandler {
	return &BotHandler{
		botRepo: botRepo,
	}
}

// CreateBotRequest represents a request to create a new bot
type CreateBotRequest struct {
	Name         string  `json:"name" validate:"required,min=3,max=100"`
	Description  string  `json:"description" validate:"max=500"`
	Temperature  float64 `json:"temperature" validate:"omitempty,gte=0,lte=2"`
	TopP         float64 `json:"top_p" validate:"omitempty,gte=0,lte=1"`
	TopK         int     `json:"top_k" validate:"omitempty,gte=1,lte=200"`
	MaxNewTokens int     `json:"max_new_tokens" validate:"omitempty,gte=32,lte=4096"`
	DoSample     bool    `json:"do_sample"`
	SystemPrompt string  `json:"system_prompt" validate:"omitempty,max=2000"`
	RAGTopK      int     `json:"rag_top_k" validate:"omitempty,gte=1,lte=10"`
	ChunkSize    int     `json:"chunk_size" validate:"omitempty,gte=100,lte=5000"`
	ChunkOverlap int     `json:"chunk_overlap" validate:"omitempty,gte=0,lte=1000"`
}

// UpdateBotRequest represents a request to update an existing bot
type UpdateBotRequest struct {
	Name         string  `json:"name" validate:"omitempty,min=3,max=100"`
	Description  string  `json:"description" validate:"omitempty,max=500"`
	Temperature  float64 `json:"temperature" validate:"omitempty,gte=0,lte=2"`
	TopP         float64 `json:"top_p" validate:"omitempty,gte=0,lte=1"`
	TopK         int     `json:"top_k" validate:"omitempty,gte=1,lte=200"`
	MaxNewTokens int     `json:"max_new_tokens" validate:"omitempty,gte=32,lte=4096"`
	DoSample     *bool   `json:"do_sample"`
	SystemPrompt string  `json:"system_prompt" validate:"omitempty,max=2000"`
	RAGTopK      int     `json:"rag_top_k" validate:"omitempty,gte=1,lte=10"`
	ChunkSize    int     `json:"chunk_size" validate:"omitempty,gte=100,lte=5000"`
	ChunkOverlap int     `json:"chunk_overlap" validate:"omitempty,gte=0,lte=1000"`
}

// CreateBot creates a new bot
func (h *BotHandler) CreateBot(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	req := new(CreateBotRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Set defaults
	if req.Temperature == 0 {
		req.Temperature = 0.75
	}
	if req.TopP == 0 {
		req.TopP = 0.92
	}
	if req.TopK == 0 {
		req.TopK = 40
	}
	if req.MaxNewTokens == 0 {
		req.MaxNewTokens = 512
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = 800
	}
	if req.ChunkOverlap == 0 {
		req.ChunkOverlap = 200
	}
	if req.SystemPrompt == "" {
		req.SystemPrompt = "You are a helpful assistant. /no_think"
	}

	bot := &database.Bot{
		ID:           uuid.New().String(),
		OwnerID:      userID,
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Config:       "{}",
		Temperature:  req.Temperature,
		TopP:         req.TopP,
		TopK:         req.TopK,
		MaxNewTokens: req.MaxNewTokens,
		DoSample:     req.DoSample,
		SystemPrompt: req.SystemPrompt,
		ChunkSize:    req.ChunkSize,
		ChunkOverlap: req.ChunkOverlap,
		IsActive:     true,
	}

	createdBot, err := h.botRepo.Create(bot)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to create bot",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdBot)
}

// GetMyBots returns all bots owned by the current user
func (h *BotHandler) GetMyBots(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	bots, err := h.botRepo.GetByOwnerID(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get bots",
		})
	}

	return c.JSON(fiber.Map{
		"bots": bots,
	})
}

// GetBot returns a specific bot (owner can see full details, others see public info)
func (h *BotHandler) GetBot(c *fiber.Ctx) error {
	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "bot_id is required",
		})
	}

	bot, err := h.botRepo.GetByID(botID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "bot not found",
		})
	}

	// Check if user is the owner
	userID, _ := auth.GetUserID(c)
	if userID == bot.OwnerID {
		// Owner sees full details
		return c.JSON(bot)
	}

	// Others see public info only
	return c.JSON(bot.ToPublic())
}

// UpdateBot updates an existing bot
func (h *BotHandler) UpdateBot(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "bot_id is required",
		})
	}

	// Check ownership
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "bot not found",
		})
	}
	if !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "you don't have permission to update this bot",
		})
	}

	// Get existing bot
	bot, err := h.botRepo.GetByID(botID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "bot not found",
		})
	}

	// Parse update request
	req := new(UpdateBotRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Update fields if provided
	if req.Name != "" {
		bot.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		bot.Description = strings.TrimSpace(req.Description)
	}
	if req.Temperature > 0 {
		bot.Temperature = req.Temperature
	}
	if req.TopP > 0 {
		bot.TopP = req.TopP
	}
	if req.TopK > 0 {
		bot.TopK = req.TopK
	}
	if req.MaxNewTokens > 0 {
		bot.MaxNewTokens = req.MaxNewTokens
	}
	if req.DoSample != nil {
		bot.DoSample = *req.DoSample
	}
	if req.SystemPrompt != "" {
		bot.SystemPrompt = req.SystemPrompt
	}
	if req.ChunkSize > 0 {
		bot.ChunkSize = req.ChunkSize
	}
	if req.ChunkOverlap >= 0 {
		bot.ChunkOverlap = req.ChunkOverlap
	}

	if err := h.botRepo.Update(bot); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to update bot",
		})
	}

	return c.JSON(bot)
}

// DeleteBot deletes a bot
func (h *BotHandler) DeleteBot(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "bot_id is required",
		})
	}

	if err := h.botRepo.Delete(botID, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to delete bot",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "bot deleted successfully",
	})
}

// GetBotDocuments returns all documents for a bot
func (h *BotHandler) GetBotDocuments(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "bot_id is required",
		})
	}

	// Check ownership
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "bot not found",
		})
	}
	if !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "you don't have permission to view this bot's documents",
		})
	}

	documents, err := h.botRepo.GetDocuments(botID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get documents",
		})
	}

	return c.JSON(fiber.Map{
		"documents": documents,
	})
}
