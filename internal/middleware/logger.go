package middleware

import (
	"context"
	"time"

	"github.com/anunay/wallet-service/internal/metrics"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type correlationIDKey struct{}

func ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, correlationID)
}

func CorrelationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if cid, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return cid
	}
	return ""
}

func GetCorrelationID(c *fiber.Ctx) string {
	if c == nil {
		return ""
	}
	return CorrelationIDFromContext(c.UserContext())
}

func NewLoggerMiddleware(log *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = c.Get("X-Request-ID")
		}
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		c.Set("X-Correlation-ID", correlationID)
		c.Set("X-Request-ID", correlationID)
		c.Locals("correlation_id", correlationID)

		// Attach correlation ID to standard Go request context
		ctx := ContextWithCorrelationID(c.UserContext(), correlationID)
		c.SetUserContext(ctx)

		err := c.Next()

		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
		status := c.Response().StatusCode()

		// Record HTTP request metrics
		metrics.GetCollector().RecordRequestWithMethod(c.Method(), status, latencyMs)

		fields := []zap.Field{
			zap.String("correlation_id", correlationID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Float64("latency_ms", latencyMs),
			zap.String("ip", c.IP()),
		}

		if status >= 500 {
			log.Error("HTTP Request", fields...)
		} else if status >= 400 {
			log.Warn("HTTP Request", fields...)
		} else {
			log.Info("HTTP Request", fields...)
		}

		return err
	}
}
