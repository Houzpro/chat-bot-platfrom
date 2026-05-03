package handlers

import (
	"backend/auth"
	"backend/database"
	"backend/pagination"

	"github.com/gofiber/fiber/v2"
)

type ConversationHandler struct {
	convRepo   *database.ConversationRepository
	botRepo    *database.BotRepository
	collabRepo *database.CollaboratorRepository
}

func NewConversationHandler(convRepo *database.ConversationRepository, botRepo *database.BotRepository, collabRepo *database.CollaboratorRepository) *ConversationHandler {
	return &ConversationHandler{
		convRepo:   convRepo,
		botRepo:    botRepo,
		collabRepo: collabRepo,
	}
}

// canAccessBot mirrors handlers.Handler.hasBotAccess — duplicated here so
// conversation_handler doesn't need an import cycle through Handler. Returns
// true for the owner or any collaborator (viewer/editor).
func (h *ConversationHandler) canAccessBot(botID, userID string) bool {
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err == nil && isOwner {
		return true
	}
	if h.collabRepo == nil {
		return false
	}
	role, err := h.collabRepo.GetRole(botID, userID)
	return err == nil && role != ""
}

// CreateConversation creates a new conversation for a bot
// POST /api/v1/conversations
func (h *ConversationHandler) CreateConversation(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		BotID string `json:"bot_id"`
		Title string `json:"title"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.BotID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}

	if !h.canAccessBot(body.BotID, userID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your bot"})
	}

	title := body.Title
	if title == "" {
		title = "New conversation"
	}

	conv := &database.Conversation{
		BotID:  body.BotID,
		UserID: &userID,
		Title:  title,
	}

	conv, err := h.convRepo.CreateConversation(conv)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create conversation"})
	}

	return c.Status(fiber.StatusCreated).JSON(conv)
}

// GetBotConversations returns all conversations for a bot
// GET /api/v1/bots/:id/conversations
func (h *ConversationHandler) GetBotConversations(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	botID := normalizeBotID(c.Params("id"))
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}

	if !h.canAccessBot(botID, userID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your bot"})
	}

	p := pagination.FromCtx(c)
	convs, total, err := h.convRepo.GetConversationsByBotIDAndUserIDPaginated(botID, userID, p.Offset(), p.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get conversations"})
	}

	return c.JSON(pagination.Build(convs, p, total))
}

// GetConversation returns a conversation with its messages
// GET /api/v1/conversations/:conv_id
func (h *ConversationHandler) GetConversation(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	convID := c.Params("conv_id")
	conv, err := h.convRepo.GetConversationByID(convID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	}

	if conv.UserID == nil || *conv.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your conversation"})
	}

	msgs, err := h.convRepo.GetMessages(convID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get messages"})
	}

	return c.JSON(fiber.Map{
		"conversation": conv,
		"messages":     msgs,
	})
}

// DeleteConversation deletes a conversation
// DELETE /api/v1/conversations/:conv_id
func (h *ConversationHandler) DeleteConversation(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	convID := c.Params("conv_id")
	conv, err := h.convRepo.GetConversationByID(convID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	}

	if conv.UserID == nil || *conv.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your conversation"})
	}

	if err := h.convRepo.DeleteConversation(convID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete conversation"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "conversation deleted"})
}

// SubmitFeedback adds or updates feedback (thumbs up/down) for a message
// POST /api/v1/messages/:message_id/feedback
func (h *ConversationHandler) SubmitFeedback(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	messageID := c.Params("message_id")
	if messageID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	var body struct {
		Rating int16 `json:"rating"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Rating != 1 && body.Rating != -1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "rating must be 1 or -1"})
	}

	// Verify message exists and belongs to user
	msg, err := h.convRepo.GetMessageByID(messageID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "message not found"})
	}

	if msg.ConversationID != nil && *msg.ConversationID != "" {
		conv, err := h.convRepo.GetConversationByID(*msg.ConversationID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		if conv.UserID == nil || *conv.UserID != userID {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your conversation"})
		}
	} else if msg.BotID != nil {
		if !h.canAccessBot(*msg.BotID, userID) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not your bot"})
		}
	}

	fb := &database.MessageFeedback{
		MessageID: messageID,
		UserID:    &userID,
		Rating:    body.Rating,
	}

	fb, err = h.convRepo.AddFeedback(fb)
	if err != nil {
		if err.Error() == "feedback already exists" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "feedback already submitted"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save feedback"})
	}

	return c.JSON(fb)
}

// GetFeedbackStats returns aggregated feedback stats for a bot
// GET /api/v1/bots/:id/feedback/stats
func (h *ConversationHandler) GetFeedbackStats(c *fiber.Ctx) error {
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

	stats, err := h.convRepo.GetFeedbackStats(botID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get feedback stats"})
	}

	return c.JSON(stats)
}

// PublicSubmitFeedback allows anonymous users to rate a message
// POST /api/v1/public/messages/:message_id/feedback
func (h *ConversationHandler) PublicSubmitFeedback(c *fiber.Ctx) error {
	messageID := c.Params("message_id")
	if messageID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid message_id"})
	}

	var body struct {
		Rating int16 `json:"rating"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Rating != 1 && body.Rating != -1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "rating must be 1 or -1"})
	}

	// Verify message exists
	_, err := h.convRepo.GetMessageByID(messageID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "message not found"})
	}

	fb := &database.MessageFeedback{
		MessageID: messageID,
		UserID:    nil, // anonymous
		Rating:    body.Rating,
	}

	fb, err = h.convRepo.AddFeedback(fb)
	if err != nil {
		if err.Error() == "feedback already exists" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "feedback already submitted"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save feedback"})
	}

	return c.JSON(fb)
}
