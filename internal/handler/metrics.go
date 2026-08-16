package handler

import (
	"strings"

	"github.com/anunay/wallet-service/internal/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

type MetricsHandler struct {
	promHandler fiber.Handler
}

func NewMetricsHandler() *MetricsHandler {
	collector := metrics.GetCollector()
	promHTTP := promhttp.HandlerFor(collector.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	})
	fastHTTPHandler := fasthttpadaptor.NewFastHTTPHandler(promHTTP)
	return &MetricsHandler{
		promHandler: func(c *fiber.Ctx) error {
			fastHTTPHandler(c.Context())
			return nil
		},
	}
}

func (h *MetricsHandler) GetMetrics(c *fiber.Ctx) error {
	format := strings.ToLower(c.Query("format"))
	acceptHeader := strings.ToLower(c.Get("Accept"))

	if format == "json" || strings.Contains(acceptHeader, "application/json") {
		snapshot := metrics.GetCollector().Snapshot()
		return c.Status(fiber.StatusOK).JSON(snapshot)
	}

	return h.promHandler(c)
}
