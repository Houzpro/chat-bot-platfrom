package handlers

import (
	"backend/auth"
	"backend/config"
	"backend/database"
	"backend/pagination"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type BotHandler struct {
	botRepo    *database.BotRepository
	collabRepo *database.CollaboratorRepository
	userRepo   *database.UserRepository
	modelRepo  *database.ModelRepository
	cfg        *config.Config
}

func NewBotHandler(botRepo *database.BotRepository, collabRepo *database.CollaboratorRepository, userRepo *database.UserRepository, modelRepo *database.ModelRepository, cfg *config.Config) *BotHandler {
	return &BotHandler{
		botRepo:    botRepo,
		collabRepo: collabRepo,
		userRepo:   userRepo,
		modelRepo:  modelRepo,
		cfg:        cfg,
	}
}

// CreateBotRequest represents a request to create a new bot
type CreateBotRequest struct {
	Name          string  `json:"name" validate:"required,min=3,max=100"`
	Description   string  `json:"description" validate:"max=500"`
	Temperature   float64 `json:"temperature" validate:"omitempty,gte=0,lte=2"`
	TopP          float64 `json:"top_p" validate:"omitempty,gte=0,lte=1"`
	TopK          int     `json:"top_k" validate:"omitempty,gte=1,lte=200"`
	MaxNewTokens  int     `json:"max_new_tokens" validate:"omitempty,gte=32,lte=8192"`
	DoSample      bool    `json:"do_sample"`
	SystemPrompt  string  `json:"system_prompt" validate:"omitempty,max=2000"`
	RAGTopK       int     `json:"rag_top_k" validate:"omitempty,gte=1,lte=10"`
	ChunkSize     int     `json:"chunk_size" validate:"omitempty,gte=100,lte=5000"`
	ChunkOverlap  int     `json:"chunk_overlap" validate:"omitempty,gte=0,lte=1000"`
	ContextWindow int     `json:"context_window" validate:"omitempty,gte=0,lte=50"`
	// ModelID is optional. Empty string ("") means "use the platform default
	// llama-cpp container". We validate access in CreateBot before saving.
	ModelID string `json:"model_id"`
}

// UpdateBotRequest represents a request to update an existing bot.
// Numeric/bool fields are pointers so an omitted value is distinguishable from 0
// (otherwise zero-values would overwrite existing settings — e.g. chunk_overlap 0).
type UpdateBotRequest struct {
	Name          string   `json:"name" validate:"omitempty,min=3,max=100"`
	Description   string   `json:"description" validate:"omitempty,max=500"`
	Temperature   *float64 `json:"temperature" validate:"omitempty,gte=0,lte=2"`
	TopP          *float64 `json:"top_p" validate:"omitempty,gte=0,lte=1"`
	TopK          *int     `json:"top_k" validate:"omitempty,gte=1,lte=200"`
	MaxNewTokens  *int     `json:"max_new_tokens" validate:"omitempty,gte=32,lte=8192"`
	DoSample      *bool    `json:"do_sample"`
	IsActive      *bool    `json:"is_active"`
	SystemPrompt  string   `json:"system_prompt" validate:"omitempty,max=2000"`
	RAGTopK       *int     `json:"rag_top_k" validate:"omitempty,gte=1,lte=10"`
	ChunkSize     *int     `json:"chunk_size" validate:"omitempty,gte=100,lte=5000"`
	ChunkOverlap  *int     `json:"chunk_overlap" validate:"omitempty,gte=0,lte=1000"`
	ContextWindow *int     `json:"context_window" validate:"omitempty,gte=0,lte=50"`
	// ModelID — pointer-to-string so we can distinguish three states:
	//   nil     → field absent in JSON, leave existing assignment untouched.
	//   *""     → caller wants to clear the assignment (use platform default).
	//   *"uuid" → caller wants to set/replace the assignment.
	ModelID *string `json:"model_id"`
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

	// Set defaults from env/config
	gen := h.cfg.Generation
	if req.Temperature == 0 {
		req.Temperature = gen.Temperature
	}
	if req.TopP == 0 {
		req.TopP = gen.TopP
	}
	if req.TopK == 0 {
		req.TopK = gen.TopK
	}
	if req.MaxNewTokens == 0 {
		req.MaxNewTokens = gen.MaxNewTokens
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = h.cfg.RAG.ChunkSize
	}
	if req.ChunkOverlap == 0 {
		req.ChunkOverlap = h.cfg.RAG.ChunkOverlap
	}
	if req.ContextWindow == 0 {
		req.ContextWindow = h.cfg.RAG.ContextWindowSize
	}
	if req.SystemPrompt == "" {
		req.SystemPrompt = gen.SystemBase
	}

	bot := &database.Bot{
		ID:            uuid.New().String(),
		OwnerID:       userID,
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Config:        "{}",
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		TopK:          req.TopK,
		MaxNewTokens:  req.MaxNewTokens,
		DoSample:      req.DoSample,
		SystemPrompt:  req.SystemPrompt,
		ChunkSize:     req.ChunkSize,
		ChunkOverlap:  req.ChunkOverlap,
		ContextWindow: req.ContextWindow,
		IsActive:      true,
	}

	// Only validate + attach a model when the caller actually supplied one.
	// An empty string keeps bot.ModelID = nil, which the RAG handler treats as
	// "use the platform default llama-cpp endpoint".
	if mid := strings.TrimSpace(req.ModelID); mid != "" {
		if h.modelRepo == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "model registry unavailable"})
		}
		allowed, _, err := h.modelRepo.CheckAccess(mid, userID)
		if err != nil || !allowed {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "model not accessible"})
		}
		bot.ModelID = &mid
	}

	createdBot, err := h.botRepo.Create(bot)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to create bot",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdBot)
}

// GetMyBots returns a page of bots accessible to the current user — both
// owned bots and bots shared with them as a collaborator. Each item carries
// a `role` field ("owner"/"editor"/"viewer") so the frontend can decide which
// management actions to expose.
// Query params: ?page=1&limit=20&search=...
func (h *BotHandler) GetMyBots(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "unauthorized",
		})
	}

	p := pagination.FromCtx(c)
	search := strings.TrimSpace(c.Query("search"))
	bots, total, err := h.botRepo.GetAccessibleBotsPaginated(userID, search, p.Offset(), p.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get bots",
		})
	}

	return c.JSON(pagination.Build(bots, p, total))
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

	// Get existing bot (checks ownership via owner_id, no is_active filter so owner can reactivate)
	bot, err := h.botRepo.GetByIDForOwner(botID, userID)
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
	if req.Temperature != nil {
		bot.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		bot.TopP = *req.TopP
	}
	if req.TopK != nil {
		bot.TopK = *req.TopK
	}
	if req.MaxNewTokens != nil {
		bot.MaxNewTokens = *req.MaxNewTokens
	}
	if req.DoSample != nil {
		bot.DoSample = *req.DoSample
	}
	if req.IsActive != nil {
		bot.IsActive = *req.IsActive
	}
	if req.SystemPrompt != "" {
		bot.SystemPrompt = req.SystemPrompt
	}
	if req.ChunkSize != nil {
		bot.ChunkSize = *req.ChunkSize
	}
	if req.ChunkOverlap != nil {
		bot.ChunkOverlap = *req.ChunkOverlap
	}
	if req.ContextWindow != nil {
		bot.ContextWindow = *req.ContextWindow
	}
	// Only react when ModelID is present in the JSON. *"" clears the
	// assignment; *"uuid" sets/replaces it (with access check).
	if req.ModelID != nil {
		mid := strings.TrimSpace(*req.ModelID)
		if mid == "" {
			bot.ModelID = nil
		} else {
			if h.modelRepo == nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "model registry unavailable"})
			}
			allowed, _, err := h.modelRepo.CheckAccess(mid, userID)
			if err != nil || !allowed {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "model not accessible"})
			}
			bot.ModelID = &mid
		}
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

	// Owner OR any collaborator (viewer/editor) can list documents — the chat
	// UI shows the document list to viewers so they understand what knowledge
	// the bot has. Editors and owners are the only ones allowed to upload/delete
	// (enforced in handlers.UploadDocumentForBot / DeleteBotDocument).
	if !h.canAccessBot(botID, userID, "viewer") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "you don't have permission to view this bot's documents",
		})
	}

	p := pagination.FromCtx(c)
	documents, total, err := h.botRepo.GetDocumentsPaginated(botID, p.Offset(), p.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get documents",
		})
	}

	return c.JSON(pagination.Build(documents, p, total))
}

// canAccessBot returns true if userID is the owner of botID, or has a
// collaborator role at least as privileged as `requiredRole`.
// requiredRole can be "viewer" (anyone with access) or "editor" (editor/owner).
func (h *BotHandler) canAccessBot(botID, userID, requiredRole string) bool {
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err == nil && isOwner {
		return true
	}
	if h.collabRepo == nil {
		return false
	}
	role, err := h.collabRepo.GetRole(botID, userID)
	if err != nil || role == "" {
		return false
	}
	if requiredRole == "editor" {
		return role == "editor"
	}
	return role == "editor" || role == "viewer"
}

// AddCollaboratorRequest is the JSON body for inviting a user.
type AddCollaboratorRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ListCollaborators returns all collaborators for a bot. Only the owner can
// see this list.
// GET /api/v1/bots/:id/collaborators
func (h *BotHandler) ListCollaborators(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil || !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the owner can manage collaborators"})
	}
	rows, err := h.collabRepo.ListByBot(botID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list collaborators"})
	}
	return c.JSON(rows)
}

// AddCollaborator invites a user (by email) to collaborate on a bot. Only
// the owner can invite. Self-invitation is rejected so the dashboard never
// double-counts the bot.
// POST /api/v1/bots/:id/collaborators
func (h *BotHandler) AddCollaborator(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id is required"})
	}

	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil || !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the owner can add collaborators"})
	}

	req := new(AddCollaboratorRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email is required"})
	}
	if req.Role != "editor" && req.Role != "viewer" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be 'editor' or 'viewer'"})
	}

	target, err := h.userRepo.GetByEmail(req.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user with this email not found"})
	}
	if target.ID == userID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "you already own this bot"})
	}

	collab, err := h.collabRepo.Add(botID, target.ID, req.Role)
	if err != nil {
		if err.Error() == "already a collaborator" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "already a collaborator"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to add collaborator"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":         collab.ID,
		"bot_id":     collab.BotID,
		"user_id":    collab.UserID,
		"role":       collab.Role,
		"email":      target.Email,
		"name":       target.Name,
		"created_at": collab.CreatedAt,
	})
}

// UpdateCollaboratorRequest body for PUT.
type UpdateCollaboratorRequest struct {
	Role string `json:"role"`
}

// UpdateCollaborator changes a collaborator's role.
// PUT /api/v1/bots/:id/collaborators/:user_id
func (h *BotHandler) UpdateCollaborator(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	botID := c.Params("id")
	targetID := c.Params("user_id")
	if botID == "" || targetID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id and user_id are required"})
	}
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil || !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the owner can manage collaborators"})
	}

	req := new(UpdateCollaboratorRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Role != "editor" && req.Role != "viewer" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be 'editor' or 'viewer'"})
	}
	if err := h.collabRepo.UpdateRole(botID, targetID, req.Role); err != nil {
		if err.Error() == "collaborator not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "collaborator not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update collaborator"})
	}
	return c.JSON(fiber.Map{"success": true})
}

// RemoveCollaborator deletes a collaborator entry.
// DELETE /api/v1/bots/:id/collaborators/:user_id
func (h *BotHandler) RemoveCollaborator(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	botID := c.Params("id")
	targetID := c.Params("user_id")
	if botID == "" || targetID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot_id and user_id are required"})
	}
	isOwner, err := h.botRepo.CheckOwnership(botID, userID)
	if err != nil || !isOwner {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only the owner can manage collaborators"})
	}
	if err := h.collabRepo.Remove(botID, targetID); err != nil {
		if err.Error() == "collaborator not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "collaborator not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to remove collaborator"})
	}
	return c.JSON(fiber.Map{"success": true})
}
