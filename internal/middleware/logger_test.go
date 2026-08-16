package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewLoggerMiddleware(t *testing.T) {
	t.Run("Generates correlation ID when missing in request", func(t *testing.T) {
		core, recorded := observer.New(zap.InfoLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewLoggerMiddleware(log))

		app.Get("/test", func(c *fiber.Ctx) error {
			cid := GetCorrelationID(c)
			if cid == "" {
				t.Error("expected non-empty correlation ID inside handler")
			}
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Header.Get("X-Correlation-ID") == "" {
			t.Errorf("expected X-Correlation-ID response header")
		}

		if len(recorded.All()) == 0 {
			t.Errorf("expected log output")
		}
	})

	t.Run("Preserves provided X-Correlation-ID header", func(t *testing.T) {
		core, recorded := observer.New(zap.InfoLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewLoggerMiddleware(log))

		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Correlation-ID", "custom-cid-999")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Header.Get("X-Correlation-ID") != "custom-cid-999" {
			t.Errorf("expected X-Correlation-ID custom-cid-999, got %s", resp.Header.Get("X-Correlation-ID"))
		}

		logs := recorded.All()
		if len(logs) == 0 {
			t.Fatalf("expected log entry")
		}
	})

	t.Run("Logs warning on HTTP 400 error status", func(t *testing.T) {
		core, recorded := observer.New(zap.WarnLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewLoggerMiddleware(log))

		app.Get("/bad", func(c *fiber.Ctx) error {
			return c.Status(http.StatusBadRequest).SendString("bad request")
		})

		req := httptest.NewRequest(http.MethodGet, "/bad", nil)
		_, _ = app.Test(req)

		logs := recorded.All()
		if len(logs) == 0 || logs[0].Level != zap.WarnLevel {
			t.Errorf("expected Warn level log for HTTP 400 status")
		}
	})

	t.Run("Logs error on HTTP 500 error status", func(t *testing.T) {
		core, recorded := observer.New(zap.ErrorLevel)
		log := zap.New(core)

		app := fiber.New()
		app.Use(NewLoggerMiddleware(log))

		app.Get("/server-err", func(c *fiber.Ctx) error {
			return c.Status(http.StatusInternalServerError).SendString("error")
		})

		req := httptest.NewRequest(http.MethodGet, "/server-err", nil)
		_, _ = app.Test(req)

		logs := recorded.All()
		if len(logs) == 0 || logs[0].Level != zap.ErrorLevel {
			t.Errorf("expected Error level log for HTTP 500 status")
		}
	})
}

func TestCorrelationIDFromContext(t *testing.T) {
	t.Run("Nil context returns empty string", func(t *testing.T) {
		if cid := CorrelationIDFromContext(nil); cid != "" {
			t.Errorf("expected empty string for nil context, got %s", cid)
		}
	})

	t.Run("Context without correlation ID returns empty string", func(t *testing.T) {
		if cid := CorrelationIDFromContext(context.Background()); cid != "" {
			t.Errorf("expected empty string, got %s", cid)
		}
	})

	t.Run("Context with correlation ID returns set value", func(t *testing.T) {
		ctx := ContextWithCorrelationID(context.Background(), "my-cid")
		if cid := CorrelationIDFromContext(ctx); cid != "my-cid" {
			t.Errorf("expected 'my-cid', got %s", cid)
		}
	})
}
