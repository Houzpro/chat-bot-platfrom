package handlers

import (
	"backend/auth"
	"backend/database"

	"github.com/gofiber/fiber/v2"
)

type AnalyticsHandler struct {
	convRepo *database.ConversationRepository
	botRepo  *database.BotRepository
}

func NewAnalyticsHandler(convRepo *database.ConversationRepository, botRepo *database.BotRepository) *AnalyticsHandler {
	return &AnalyticsHandler{
		convRepo: convRepo,
		botRepo:  botRepo,
	}
}

// GetBotAnalytics returns aggregated analytics for a bot (owner only).
// GET /api/v1/bots/:id/analytics
func (h *AnalyticsHandler) GetBotAnalytics(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	botID := normalizeBotID(c.Params("id"))
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}

	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil || !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your bot"})
	}

	analytics, err := h.convRepo.GetBotAnalytics(botID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get analytics"})
	}

	return c.JSON(analytics)
}
