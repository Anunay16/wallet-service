package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewRecoveryMiddleware(t *testing.T) {
	t.Run("Normal request without panic passes through", func(t *testing.T) {
		app := fiber.New()
		log := zap.NewNop()
		app.Use(NewRecoveryMiddleware(log))

		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "ok" {
			t.Errorf("expected body 'ok', got %q", string(body))
		}
	})

	t.Run("Recovers from string panic", func(t *testing.T) {
		core, recorded := observer.New(zap.ErrorLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewRecoveryMiddleware(log))

		app.Get("/panic-string", func(c *fiber.Ctx) error {
			panic("test string panic")
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-string", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", resp.StatusCode)
		}

		logs := recorded.All()
		if len(logs) == 0 {
			t.Fatal("expected at least one log entry for panic recovery")
		}
		if logs[0].Message != "Unhandled panic recovered in request handler" {
			t.Errorf("unexpected log message: %s", logs[0].Message)
		}
	})

	t.Run("Recovers from error object panic", func(t *testing.T) {
		core, recorded := observer.New(zap.ErrorLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewRecoveryMiddleware(log))

		app.Get("/panic-error", func(c *fiber.Ctx) error {
			panic(errors.New("database connection failed"))
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-error", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", resp.StatusCode)
		}

		logs := recorded.All()
		if len(logs) == 0 {
			t.Fatal("expected log entry for panic recovery")
		}
	})

	t.Run("Recovers from non-error struct panic with nil logger", func(t *testing.T) {
		app := fiber.New()
		app.Use(NewRecoveryMiddleware(nil))

		app.Get("/panic-int", func(c *fiber.Ctx) error {
			panic(40404)
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-int", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", resp.StatusCode)
		}
	})

	t.Run("Recovers and captures correlation ID when logger middleware is present", func(t *testing.T) {
		core, recorded := observer.New(zap.ErrorLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewRecoveryMiddleware(log))
		app.Use(NewLoggerMiddleware(log))

		app.Get("/panic-with-cid", func(c *fiber.Ctx) error {
			panic("panic with correlation id")
		})

		req := httptest.NewRequest(http.MethodGet, "/panic-with-cid", nil)
		req.Header.Set("X-Correlation-ID", "test-corr-123")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", resp.StatusCode)
		}

		logs := recorded.All()
		if len(logs) == 0 {
			t.Fatal("expected log entry")
		}

		foundCID := false
		for _, f := range logs[0].Context {
			if f.Key == "correlation_id" && f.String == "test-corr-123" {
				foundCID = true
				break
			}
		}
		if !foundCID {
			t.Errorf("expected correlation_id 'test-corr-123' in log fields, got context: %v", logs[0].Context)
		}
	})
}
