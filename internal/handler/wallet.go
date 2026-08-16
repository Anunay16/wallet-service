package handler

import (
	"context"
	"errors"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type walletService interface {
	GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error)
	GetWalletByID(ctx context.Context, walletID uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error)
}

type WalletHandler struct {
	walletService walletService
	log           *zap.Logger
}

type WalletHandlerOption func(*WalletHandler)

func WithWalletHandlerLogger(log *zap.Logger) WalletHandlerOption {
	return func(h *WalletHandler) {
		if log != nil {
			h.log = log
		}
	}
}

func NewWalletHandler(walletService walletService, opts ...WalletHandlerOption) *WalletHandler {
	h := &WalletHandler{
		walletService: walletService,
		log:           zap.NewNop(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *WalletHandler) GetOrCreateWallet(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	walletResp, err := h.walletService.GetOrCreateWallet(c.Context(), userID)
	if err != nil {
		h.log.Error("GetOrCreateWallet internal error",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("user_id", userID.String()),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(walletResp)
}

func (h *WalletHandler) GetWalletByID(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
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
		h.log.Error("GetWalletByID internal error",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("wallet_id", walletID.String()),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(walletResp)
}
