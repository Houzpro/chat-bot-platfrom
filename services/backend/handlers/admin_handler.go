package handlers

import (
	"backend/auth"
	"backend/database"
	"backend/pagination"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// AdminHandler exposes platform-admin endpoints. Routes under /api/v1/admin/*
// are gated by auth.AdminMiddleware so callers reaching these handlers are
// already known to be admins — no per-handler role checks here.
type AdminHandler struct {
	userRepo *database.UserRepository
	botRepo  *database.BotRepository
	convRepo *database.ConversationRepository
}

func NewAdminHandler(userRepo *database.UserRepository, botRepo *database.BotRepository, convRepo *database.ConversationRepository) *AdminHandler {
	return &AdminHandler{
		userRepo: userRepo,
		botRepo:  botRepo,
		convRepo: convRepo,
	}
}

// GetStats returns platform-wide totals for the admin dashboard's metric cards.
// GET /api/v1/admin/stats
func (h *AdminHandler) GetStats(c *fiber.Ctx) error {
	users, _ := h.userRepo.CountUsers()
	bots, _ := h.botRepo.CountAllBots()
	convs, _ := h.convRepo.CountAllConversations()
	msgs, _ := h.convRepo.CountAllMessages()

	return c.JSON(fiber.Map{
		"total_users":         users,
		"total_bots":          bots,
		"total_conversations": convs,
		"total_messages":      msgs,
	})
}

// ListUsers returns a page of all users.
// GET /api/v1/admin/users?page=&limit=&search=
func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	p := pagination.FromCtx(c)
	search := strings.TrimSpace(c.Query("search"))
	users, total, err := h.userRepo.ListUsersPaginated(search, p.Offset(), p.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list users"})
	}
	return c.JSON(pagination.Build(users, p, total))
}

// SetUserRoleRequest is the body for PUT /admin/users/:id/role.
type SetUserRoleRequest struct {
	Role string `json:"role"`
}

// SetUserRole promotes/demotes a user. Admins cannot demote themselves to
// avoid locking the platform out of admin access; if there's only one admin
// left, demotion would leave no one able to manage the system.
// PUT /api/v1/admin/users/:id/role
func (h *AdminHandler) SetUserRole(c *fiber.Ctx) error {
	currentUserID, _ := auth.GetUserID(c)
	targetID := c.Params("id")
	if targetID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user id is required"})
	}

	req := new(SetUserRoleRequest)
	if err := c.BodyParser(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Role != "user" && req.Role != "admin" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "role must be 'user' or 'admin'"})
	}

	if targetID == currentUserID && req.Role != "admin" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot demote yourself"})
	}

	if err := h.userRepo.SetRole(targetID, req.Role); err != nil {
		if err.Error() == "user not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update role"})
	}

	return c.JSON(fiber.Map{"success": true, "user_id": targetID, "role": req.Role})
}

// DeleteUser permanently removes a user and (via FK cascades) all their data.
// Admins cannot delete themselves — same rationale as demotion.
// DELETE /api/v1/admin/users/:id
func (h *AdminHandler) DeleteUser(c *fiber.Ctx) error {
	currentUserID, _ := auth.GetUserID(c)
	targetID := c.Params("id")
	if targetID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user id is required"})
	}
	if targetID == currentUserID {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete yourself"})
	}
	if err := h.userRepo.DeleteUser(targetID); err != nil {
		if err.Error() == "user not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete user"})
	}
	return c.JSON(fiber.Map{"success": true})
}

// ListBots returns a page of all bots in the system, with their owners.
// GET /api/v1/admin/bots?page=&limit=&search=
func (h *AdminHandler) ListBots(c *fiber.Ctx) error {
	p := pagination.FromCtx(c)
	search := strings.TrimSpace(c.Query("search"))
	bots, total, err := h.botRepo.ListAllBotsPaginated(search, p.Offset(), p.Limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list bots"})
	}
	return c.JSON(pagination.Build(bots, p, total))
}

// DeleteBot removes a bot regardless of owner. Cascades through FKs delete
// related documents/conversations/messages.
// DELETE /api/v1/admin/bots/:id
func (h *AdminHandler) DeleteBot(c *fiber.Ctx) error {
	botID := c.Params("id")
	if botID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bot id is required"})
	}
	if err := h.botRepo.AdminDeleteBot(botID); err != nil {
		if err.Error() == "bot not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "bot not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete bot"})
	}
	return c.JSON(fiber.Map{"success": true})
}
