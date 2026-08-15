package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func NewLoggerMiddleware(log *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		reqID := c.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
			c.Set("X-Request-ID", reqID)
		}

		err := c.Next()

		latency := time.Since(start).Milliseconds()
		status := c.Response().StatusCode()

		fields := []zap.Field{
			zap.String("request_id", reqID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Int64("latency_ms", latency),
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
