package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type transferService interface {
	InitiateTransfer(ctx context.Context, req domain.TransferRequest, callerUserID uuid.UUID) (*domain.IdempotencyRecord, error)
	GetTransferByID(ctx context.Context, transferID uuid.UUID, callerUserID uuid.UUID) (*domain.Transfer, error)
}

type TransferHandler struct {
	transferService transferService
	log             *zap.Logger
}

type TransferHandlerOption func(*TransferHandler)

func WithTransferHandlerLogger(log *zap.Logger) TransferHandlerOption {
	return func(h *TransferHandler) {
		if log != nil {
			h.log = log
		}
	}
}

func NewTransferHandler(transferService transferService, opts ...TransferHandlerOption) *TransferHandler {
	h := &TransferHandler{
		transferService: transferService,
		log:             zap.NewNop(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *TransferHandler) InitiateTransfer(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req domain.TransferRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	idemRecord, err := h.transferService.InitiateTransfer(c.Context(), req, userID)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidAmount) || errors.Is(err, domain.ErrSameWalletTransfer) || errors.Is(err, domain.ErrEmptyIdempotencyKey) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, domain.ErrWalletNotFound) || errors.Is(err, domain.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "recipient or sender user/wallet not found"})
		}
		if errors.Is(err, domain.ErrForbiddenWalletAccess) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		if errors.Is(err, domain.ErrIdempotencyConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "idempotency_key_conflict"})
		}
		if errors.Is(err, domain.ErrInsufficientFunds) && idemRecord != nil {
			c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
			return c.Status(idemRecord.ResponseStatus).Send(idemRecord.ResponseBody)
		}
		if idemRecord == nil {
			h.log.Error("InitiateTransfer internal error",
				zap.Error(err),
				zap.String("correlation_id", cid),
				zap.String("user_id", userID.String()),
			)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	if idemRecord != nil {
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Status(idemRecord.ResponseStatus).Send(idemRecord.ResponseBody)
	}

	h.log.Error("InitiateTransfer unexpected state",
		zap.String("correlation_id", cid),
		zap.String("user_id", userID.String()),
	)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unexpected transfer state"})
}

func (h *TransferHandler) GetTransferByID(c *fiber.Ctx) error {
	cid := middleware.GetCorrelationID(c)
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	idParam := c.Params("id")
	transferID, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid transfer id format"})
	}

	transfer, err := h.transferService.GetTransferByID(c.Context(), transferID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrTransferNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "transfer record not found"})
		}
		if errors.Is(err, domain.ErrForbiddenWalletAccess) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		h.log.Error("GetTransferByID internal error",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("transfer_id", transferID.String()),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	resp := domain.TransferResponse{
		ID:            transfer.ID,
		FromWalletID:  transfer.FromWalletID,
		ToWalletID:    transfer.ToWalletID,
		Amount:        transfer.Amount,
		Status:        transfer.Status,
		FailureReason: transfer.FailureReason,
		CreatedAt:     transfer.CreatedAt,
		UpdatedAt:     transfer.UpdatedAt,
	}

	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	bytes, _ := json.Marshal(resp)
	return c.Status(fiber.StatusOK).Send(bytes)
}
