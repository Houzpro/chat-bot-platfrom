package handlers

import (
	"backend/auth"
	"backend/database"
	"backend/services"

	"github.com/gofiber/fiber/v2"
)

// ModelHandler exposes the model registry to authenticated users.
//
// Read endpoints work with the registry alone (`modelRepo`). Deploy/stop
// additionally drive the Docker daemon through `containerMgr`. The latter
// can be nil in environments where the docker socket isn't available
// (e.g. unit tests, CI) — in that case we return a clear 503 so callers
// don't get a confusing nil-pointer panic.
type ModelHandler struct {
	modelRepo    *database.ModelRepository
	containerMgr *services.ContainerManager
}

func NewModelHandler(modelRepo *database.ModelRepository, containerMgr *services.ContainerManager) *ModelHandler {
	return &ModelHandler{modelRepo: modelRepo, containerMgr: containerMgr}
}

// ListModels returns every model the user is allowed to assign: all base
// models + their own finetuned models. Forbidden rows are simply absent —
// we never return a 403 here, the user just sees fewer options.
// GET /api/v1/models
func (h *ModelHandler) ListModels(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	models, err := h.modelRepo.GetAvailableModels(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list models"})
	}
	return c.JSON(fiber.Map{"items": models})
}

// GetModel returns a single model. Base models are world-readable; finetuned
// models 404 for non-owners (we 404 instead of 403 so the existence of a
// foreign user's model is not leaked).
// GET /api/v1/models/:id
func (h *ModelHandler) GetModel(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "model id required"})
	}
	allowed, m, err := h.modelRepo.CheckAccess(id, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "model not found"})
	}
	if !allowed {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "model not found"})
	}
	return c.JSON(m)
}

// canDeploy gates Deploy/Stop/Delete to legitimate owners. Base models
// without an owner are protected: the platform-default container is
// owned by docker-compose, not us, so we refuse to touch it.
func (h *ModelHandler) canDeploy(m *database.Model, userID string) bool {
	if m.Type == "base" {
		// Base models in the registry are seeded by SeedBaseModels with
		// owner_id = NULL. Phase-2 user-stage 3 explicitly asked to allow
		// deploying a base model as its own container for testing — but we
		// still prevent acting on the *platform-default* container by name.
		if m.ContainerName == "chatbot-llama-cpp" {
			return false
		}
		// Fall through: any user may spin up an extra container for a
		// shared base model. The container itself only consumes that
		// user's port slot in the pool.
		return true
	}
	if m.OwnerID == nil {
		return false
	}
	return *m.OwnerID == userID
}

// DeployModel boots a llama.cpp container for the model. Idempotent — if
// the container is already running we just refresh the registry row.
// POST /api/v1/models/:id/deploy
func (h *ModelHandler) DeployModel(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.containerMgr == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "container manager unavailable"})
	}
	id := c.Params("id")
	allowed, m, err := h.modelRepo.CheckAccess(id, userID)
	if err != nil || !allowed {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "model not found"})
	}
	if !h.canDeploy(m, userID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot deploy this model"})
	}

	// Mark the row as deploying *before* the long Docker call so concurrent
	// double-clicks see the in-progress state. The actual container creation
	// is synchronous below; on success we flip to 'running'.
	m.Status = "deploying"
	if err := h.modelRepo.Update(m); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update model"})
	}

	// Pass the full model list so the manager can scan for occupied ports
	// — using the registry as source of truth keeps port assignment
	// consistent across backend restarts.
	allModels, _ := h.modelRepo.GetAvailableModels(userID)

	containerName, port, endpoint, err := h.containerMgr.Deploy(c.Context(), m, allModels)
	if err != nil {
		m.Status = "error"
		_ = h.modelRepo.Update(m)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	m.ContainerName = containerName
	m.ContainerPort = port
	m.EndpointURL = endpoint
	m.Status = "running"
	if err := h.modelRepo.Update(m); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to persist deploy state"})
	}
	return c.JSON(m)
}

// StopModel tears down the model's container and clears endpoint metadata
// so the chat router falls back to the platform default.
// POST /api/v1/models/:id/stop
func (h *ModelHandler) StopModel(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.containerMgr == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "container manager unavailable"})
	}
	id := c.Params("id")
	allowed, m, err := h.modelRepo.CheckAccess(id, userID)
	if err != nil || !allowed {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "model not found"})
	}
	if !h.canDeploy(m, userID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot stop this model"})
	}
	if err := h.containerMgr.Stop(c.Context(), m.ContainerName); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	m.Status = "stopped"
	m.ContainerName = ""
	m.ContainerPort = 0
	m.EndpointURL = ""
	if err := h.modelRepo.Update(m); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to persist stop state"})
	}
	return c.JSON(m)
}

// DeleteModel removes a finetuned model. Refuses base models so a click
// can't destroy the platform's only LLM. Container is stopped first so we
// don't leak a running daemon process.
// DELETE /api/v1/models/:id
func (h *ModelHandler) DeleteModel(c *fiber.Ctx) error {
	userID, ok := auth.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")
	allowed, m, err := h.modelRepo.CheckAccess(id, userID)
	if err != nil || !allowed {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "model not found"})
	}
	if m.Type == "base" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot delete base model"})
	}
	if m.OwnerID == nil || *m.OwnerID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "not owner"})
	}
	// Best-effort container teardown; we delete the row even if Docker
	// reports an error so the user isn't stuck with an undeletable model.
	if h.containerMgr != nil && m.ContainerName != "" {
		_ = h.containerMgr.Stop(c.Context(), m.ContainerName)
	}
	if err := h.modelRepo.Delete(id, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}
