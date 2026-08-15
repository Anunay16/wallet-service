package handler

import (
	"context"
	"errors"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/gofiber/fiber/v2"
)

type authService interface {
	Register(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error)
	Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error)
}

type AuthHandler struct {
	authService authService
}

func NewAuthHandler(authService authService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req domain.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	userResp, err := h.authService.Register(c.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "user already exists"})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(userResp)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req domain.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	loginResp, err := h.authService.Login(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}

	return c.Status(fiber.StatusOK).JSON(loginResp)
}
