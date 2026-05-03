package auth

import (
	"github.com/gofiber/fiber/v2"
)

// UserLookup is the minimal contract this middleware needs from a user
// repository. Defined as an interface (instead of importing the concrete
// *database.UserRepository) to avoid an import cycle: auth → database →
// auth would form a loop. Only main.go knows the concrete type and passes
// it in.
type UserLookup interface {
	GetRole(userID string) (string, error)
}

// AdminMiddleware blocks requests where the authenticated user does not have
// role='admin'. Must run AFTER the main JWT Middleware so user_id is in the
// Fiber context. Returns 403 (not 401) to make it clear the token was valid
// but the user is unprivileged.
func AdminMiddleware(users UserLookup) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, ok := GetUserID(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		role, err := users.GetRole(userID)
		if err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
		}
		c.Locals("user_role", role)
		return c.Next()
	}
}
