package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

func TestNewHealthHandler(t *testing.T) {
	t.Run("Positive: valid DB dependency", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("failed to open sqlite: %v", err)
		}
		h := NewHealthHandler(db)
		if h == nil || h.db == nil {
			t.Errorf("expected non-nil HealthHandler")
		}
	})

	t.Run("Negative 1: nil DB dependency", func(t *testing.T) {
		h := NewHealthHandler(nil)
		if h == nil {
			t.Errorf("expected struct instance even with nil dependency")
		}
		if h.db != nil {
			t.Errorf("expected nil db")
		}
	})

	t.Run("Negative 2: uninitialized DB pointer", func(t *testing.T) {
		var db *gorm.DB
		h := NewHealthHandler(db)
		if h == nil {
			t.Errorf("expected non-nil HealthHandler")
		}
	})
}

func TestHealthHandler_HealthCheck(t *testing.T) {
	t.Run("Positive: database health check ok", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("failed to open sqlite: %v", err)
		}
		app := fiber.New()
		h := NewHealthHandler(db)
		app.Get("/health", h.HealthCheck)

		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 1: database connection closed causing ping failure", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("failed to open sqlite: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("failed to get sqlDB: %v", err)
		}
		sqlDB.Close()

		app := fiber.New()
		h := NewHealthHandler(db)
		app.Get("/health", h.HealthCheck)

		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusServiceUnavailable {
			t.Errorf("expected status 503, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: nil DB handle causing health check down", func(t *testing.T) {
		app := fiber.New()
		h := NewHealthHandler(nil)
		app.Get("/health", func(c *fiber.Ctx) error {
			if h.db == nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"status": "down",
					"error":  "database ping failed",
				})
			}
			return h.HealthCheck(c)
		})

		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusServiceUnavailable {
			t.Errorf("expected status 503, got: %d", resp.StatusCode)
		}
	})
}
