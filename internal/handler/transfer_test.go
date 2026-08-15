package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type mockTransferService struct {
	initiateTransferFunc func(ctx context.Context, req domain.TransferRequest, callerUserID uuid.UUID) (*domain.IdempotencyRecord, error)
	getTransferByIDFunc  func(ctx context.Context, transferID uuid.UUID, callerUserID uuid.UUID) (*domain.Transfer, error)
}

func (m *mockTransferService) InitiateTransfer(ctx context.Context, req domain.TransferRequest, callerUserID uuid.UUID) (*domain.IdempotencyRecord, error) {
	if m.initiateTransferFunc != nil {
		return m.initiateTransferFunc(ctx, req, callerUserID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTransferService) GetTransferByID(ctx context.Context, transferID uuid.UUID, callerUserID uuid.UUID) (*domain.Transfer, error) {
	if m.getTransferByIDFunc != nil {
		return m.getTransferByIDFunc(ctx, transferID, callerUserID)
	}
	return nil, errors.New("not implemented")
}

func TestNewTransferHandler(t *testing.T) {
	t.Run("Positive: valid transfer service dependency", func(t *testing.T) {
		mockSvc := &mockTransferService{}
		h := NewTransferHandler(mockSvc)
		if h == nil || h.transferService == nil {
			t.Errorf("expected non-nil TransferHandler")
		}
	})

	t.Run("Negative 1: nil transfer service dependency", func(t *testing.T) {
		h := NewTransferHandler(nil)
		if h == nil {
			t.Errorf("expected struct instance even with nil dependency")
		}
		if h.transferService != nil {
			t.Errorf("expected nil transferService")
		}
	})

	t.Run("Negative 2: uninitialized mock transfer service dependency", func(t *testing.T) {
		var mockSvc *mockTransferService
		h := NewTransferHandler(mockSvc)
		if h == nil {
			t.Errorf("expected non-nil TransferHandler")
		}
	})
}

func TestTransferHandler_InitiateTransfer(t *testing.T) {
	callerID := uuid.New()

	t.Run("Positive: successful transfer initiation", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{
			initiateTransferFunc: func(ctx context.Context, req domain.TransferRequest, callerUserID uuid.UUID) (*domain.IdempotencyRecord, error) {
				return &domain.IdempotencyRecord{
					ResponseStatus: 201,
					ResponseBody:   json.RawMessage(`{"status":"completed"}`),
				}, nil
			},
		}
		h := NewTransferHandler(mockSvc)
		app.Post("/transfers", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.InitiateTransfer(c)
		})

		body, _ := json.Marshal(domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         1000,
			IdempotencyKey: "key-123",
		})
		req := httptest.NewRequest("POST", "/transfers", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			t.Errorf("expected status 201, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 1: unauthorized request without user_id in context", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{}
		h := NewTransferHandler(mockSvc)
		app.Post("/transfers", h.InitiateTransfer)

		body, _ := json.Marshal(domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         1000,
			IdempotencyKey: "key-123",
		})
		req := httptest.NewRequest("POST", "/transfers", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected status 401, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: invalid transfer amount error", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{
			initiateTransferFunc: func(ctx context.Context, req domain.TransferRequest, callerUserID uuid.UUID) (*domain.IdempotencyRecord, error) {
				return nil, domain.ErrInvalidAmount
			},
		}
		h := NewTransferHandler(mockSvc)
		app.Post("/transfers", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.InitiateTransfer(c)
		})

		body, _ := json.Marshal(domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         -100,
			IdempotencyKey: "key-123",
		})
		req := httptest.NewRequest("POST", "/transfers", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnprocessableEntity {
			t.Errorf("expected status 422, got: %d", resp.StatusCode)
		}
	})
}

func TestTransferHandler_GetTransferByID(t *testing.T) {
	callerID := uuid.New()
	transferID := uuid.New()

	t.Run("Positive: get transfer by ID successfully", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{
			getTransferByIDFunc: func(ctx context.Context, tid uuid.UUID, callerUserID uuid.UUID) (*domain.Transfer, error) {
				return &domain.Transfer{
					ID:           tid,
					FromWalletID: uuid.New(),
					ToWalletID:   uuid.New(),
					Amount:       1000,
					Status:       domain.TransferStatusCompleted,
				}, nil
			},
		}
		h := NewTransferHandler(mockSvc)
		app.Get("/transfers/:id", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetTransferByID(c)
		})

		req := httptest.NewRequest("GET", "/transfers/"+transferID.String(), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 1: unauthorized request without user_id", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{}
		h := NewTransferHandler(mockSvc)
		app.Get("/transfers/:id", h.GetTransferByID)

		req := httptest.NewRequest("GET", "/transfers/"+transferID.String(), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected status 401, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: invalid transfer ID format", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockTransferService{}
		h := NewTransferHandler(mockSvc)
		app.Get("/transfers/:id", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetTransferByID(c)
		})

		req := httptest.NewRequest("GET", "/transfers/not-a-valid-uuid", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}
	})
}
