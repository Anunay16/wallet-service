package handler

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db *gorm.DB
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) HealthCheck(c *fiber.Ctx) error {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(c.Context()) != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "down",
			"error":  "database ping failed",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "ok",
	})
}
