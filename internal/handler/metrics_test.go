package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anunay/wallet-service/internal/metrics"
	"github.com/gofiber/fiber/v2"
)

func TestMetricsHandler(t *testing.T) {
	app := fiber.New()
	metricsHdlr := NewMetricsHandler()
	app.Get("/metrics", metricsHdlr.GetMetrics)

	// Record a sample request
	metrics.GetCollector().RecordRequest(200, 12.5)
	metrics.GetCollector().IncTransfersCreated()

	t.Run("GET /metrics Prometheus text default", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed test request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "http_requests_total") {
			t.Errorf("expected prometheus text output, got %s", bodyStr)
		}
	})

	t.Run("GET /metrics?format=json", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics?format=json", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed test request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("expected application/json content type, got %s", contentType)
		}
	})
}
