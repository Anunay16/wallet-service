package handler

import (
	"context"
	"errors"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type walletService interface {
	GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error)
	GetWalletByID(ctx context.Context, walletID uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error)
}

type WalletHandler struct {
	walletService walletService
}

func NewWalletHandler(walletService walletService) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

func (h *WalletHandler) GetOrCreateWallet(c *fiber.Ctx) error {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	walletResp, err := h.walletService.GetOrCreateWallet(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(walletResp)
}

func (h *WalletHandler) GetWalletByID(c *fiber.Ctx) error {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	idParam := c.Params("id")
	walletID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid wallet id format"})
	}

	walletResp, err := h.walletService.GetWalletByID(c.Context(), walletID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrWalletNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "wallet not found"})
		}
		if errors.Is(err, domain.ErrForbiddenWalletAccess) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(walletResp)
}
