package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// NewRecoveryMiddleware creates a Fiber middleware that recovers from any panics during request handling.
// It logs panic details and full stack trace using the provided zap.Logger and returns an HTTP 500 Internal Server Error response.
func NewRecoveryMiddleware(log *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				var err error
				if e, ok := r.(error); ok {
					err = e
				} else {
					err = fmt.Errorf("%v", r)
				}

				stack := debug.Stack()
				correlationID := GetCorrelationID(c)
				if correlationID == "" && c != nil {
					correlationID = c.Get("X-Correlation-ID")
				}
				if correlationID == "" && c != nil {
					correlationID = c.Get("X-Request-ID")
				}

				fields := []zap.Field{
					zap.Error(err),
					zap.String("panic", fmt.Sprintf("%v", r)),
					zap.String("stack", string(stack)),
					zap.String("method", c.Method()),
					zap.String("path", c.Path()),
					zap.String("ip", c.IP()),
				}
				if correlationID != "" {
					fields = append(fields, zap.String("correlation_id", correlationID))
				}

				if log != nil {
					log.Error("Unhandled panic recovered in request handler", fields...)
				}

				_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal Server Error",
				})
			}
		}()

		return c.Next()
	}
}
