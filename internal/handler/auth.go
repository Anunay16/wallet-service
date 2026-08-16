package handler

import (
	"context"
	"errors"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

type authService interface {
	Register(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error)
	Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error)
}

type AuthHandler struct {
	authService authService
	log         *zap.Logger
}

type AuthHandlerOption func(*AuthHandler)

func WithAuthHandlerLogger(log *zap.Logger) AuthHandlerOption {
	return func(h *AuthHandler) {
		if log != nil {
			h.log = log
		}
	}
}

func NewAuthHandler(authService authService, opts ...AuthHandlerOption) *AuthHandler {
	h := &AuthHandler{
		authService: authService,
		log:         zap.NewNop(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
	var req domain.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	userResp, err := h.authService.Register(c.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already exists"})
		}
		h.log.Error("User registration failed",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("username", req.Username),
		)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(userResp)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
	var req domain.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	loginResp, err := h.authService.Login(c.Context(), req)
	if err != nil {
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			h.log.Error("User login internal error",
				zap.Error(err),
				zap.String("correlation_id", cid),
				zap.String("username", req.Username),
			)
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}

	return c.Status(fiber.StatusOK).JSON(loginResp)
}
