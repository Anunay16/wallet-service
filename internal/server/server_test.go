package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/domain"
	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestInitializeServer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	_ = db.AutoMigrate(&domain.User{}, &domain.Wallet{}, &domain.Transfer{}, &domain.IdempotencyRecord{})

	secret := "test-secret"
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: "8080",
		},
		Auth: config.AuthConfig{
			JWTSecret: secret,
		},
	}
	log := zap.NewNop()

	srv := InitializeServer(db, cfg, log)
	if srv == nil || srv.App == nil {
		t.Fatal("expected non-nil server and app")
	}

	t.Run("Public route /health is wired and returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for /health, got %d", resp.StatusCode)
		}
	})

	t.Run("Public route /metrics is wired and returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for /metrics, got %d", resp.StatusCode)
		}
	})

	t.Run("Protected route /wallets blocks unauthenticated request with 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/wallets", nil)
		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauthenticated /wallets, got %d", resp.StatusCode)
		}
	})

	t.Run("Recovery middleware handles route panics and returns 500", func(t *testing.T) {
		srv.App.Get("/test-panic", func(c *fiber.Ctx) error {
			panic("simulated route panic")
		})

		userID := uuid.New().String()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": userID,
			"exp":     time.Now().Add(time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString([]byte(secret))

		req := httptest.NewRequest(http.MethodGet, "/test-panic", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", resp.StatusCode)
		}
	})

	t.Run("Custom error handler converts Fiber errors to formatted JSON", func(t *testing.T) {
		srv.App.Get("/test-custom-error", func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request data")
		})

		userID := uuid.New().String()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": userID,
			"exp":     time.Now().Add(time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString([]byte(secret))

		req := httptest.NewRequest(http.MethodGet, "/test-custom-error", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		resp, err := srv.App.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != `{"error":"invalid request data"}` {
			t.Errorf("expected custom error JSON body, got %s", string(body))
		}
	})
}
