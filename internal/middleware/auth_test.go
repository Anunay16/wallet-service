package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestNewAuthMiddleware(t *testing.T) {
	secret := "my-secret-key"

	t.Run("Positive: valid Bearer token populates user_id", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))

		var extractedID uuid.UUID
		var ok bool
		app.Get("/protected", func(c *fiber.Ctx) error {
			extractedID, ok = GetUserID(c)
			return c.SendString("success")
		})

		expectedUserID := uuid.New()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": expectedUserID.String(),
			"exp":     time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("failed to sign token: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if !ok || extractedID != expectedUserID {
			t.Errorf("expected extracted user_id %s, got %s", expectedUserID, extractedID)
		}
	})

	t.Run("Negative: missing Authorization header", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))
		app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Negative: malformed header format", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))
		app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Negative: invalid signature", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))
		app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": uuid.New().String(),
		})
		tokenString, _ := token.SignedString([]byte("wrong-secret"))

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Negative: missing user_id claim in token", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))
		app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"email": "test@example.com",
		})
		tokenString, _ := token.SignedString([]byte(secret))

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Negative: invalid UUID format in user_id claim", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewAuthMiddleware(secret))
		app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": "not-a-valid-uuid",
		})
		tokenString, _ := token.SignedString([]byte(secret))

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", resp.StatusCode)
		}
	})
}

func TestGetUserID(t *testing.T) {
	t.Run("Nil fiber context returns false", func(t *testing.T) {
		_, ok := GetUserID(nil)
		if ok {
			t.Errorf("expected false for nil context")
		}
	})
}
