package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type mockWalletService struct {
	getOrCreateWalletFunc func(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error)
	getWalletByIDFunc     func(ctx context.Context, walletID uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error)
}

func (m *mockWalletService) GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error) {
	if m.getOrCreateWalletFunc != nil {
		return m.getOrCreateWalletFunc(ctx, userID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockWalletService) GetWalletByID(ctx context.Context, walletID uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error) {
	if m.getWalletByIDFunc != nil {
		return m.getWalletByIDFunc(ctx, walletID, callerUserID)
	}
	return nil, errors.New("not implemented")
}

func TestNewWalletHandler(t *testing.T) {
	t.Run("Positive: valid wallet service dependency", func(t *testing.T) {
		mockSvc := &mockWalletService{}
		h := NewWalletHandler(mockSvc)
		if h == nil || h.walletService == nil {
			t.Errorf("expected non-nil WalletHandler")
		}
	})

	t.Run("Negative 1: nil wallet service dependency", func(t *testing.T) {
		h := NewWalletHandler(nil)
		if h == nil {
			t.Fatalf("expected struct instance even with nil dependency")
		}
		if h.walletService != nil {
			t.Errorf("expected nil walletService")
		}
	})

	t.Run("Negative 2: uninitialized mock wallet service dependency", func(t *testing.T) {
		var mockSvc *mockWalletService
		h := NewWalletHandler(mockSvc)
		if h == nil {
			t.Errorf("expected non-nil WalletHandler")
		}
	})
}

func TestWalletHandler_GetOrCreateWallet(t *testing.T) {
	callerID := uuid.New()

	t.Run("Positive: get or create wallet successfully", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockWalletService{
			getOrCreateWalletFunc: func(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error) {
				return &domain.WalletResponse{
					ID:        uuid.New(),
					Username:  "alice",
					Balance:   1000000,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetOrCreateWallet(c)
		})

		req := httptest.NewRequest("GET", "/wallets", nil)
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
		mockSvc := &mockWalletService{}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets", h.GetOrCreateWallet)

		req := httptest.NewRequest("GET", "/wallets", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected status 401, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: wallet service error", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockWalletService{
			getOrCreateWalletFunc: func(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error) {
				return nil, errors.New("db error")
			},
		}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetOrCreateWallet(c)
		})

		req := httptest.NewRequest("GET", "/wallets", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Errorf("expected status 500, got: %d", resp.StatusCode)
		}
	})
}

func TestWalletHandler_GetWalletByID(t *testing.T) {
	callerID := uuid.New()
	walletID := uuid.New()

	t.Run("Positive: get wallet by ID successfully", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockWalletService{
			getWalletByIDFunc: func(ctx context.Context, wid uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error) {
				return &domain.WalletResponse{
					ID:        wid,
					Username:  "alice",
					Balance:   1000000,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets/:id", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetWalletByID(c)
		})

		req := httptest.NewRequest("GET", "/wallets/"+walletID.String(), nil)
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
		mockSvc := &mockWalletService{}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets/:id", h.GetWalletByID)

		req := httptest.NewRequest("GET", "/wallets/"+walletID.String(), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected status 401, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: invalid wallet ID format", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockWalletService{}
		h := NewWalletHandler(mockSvc)
		app.Get("/wallets/:id", func(c *fiber.Ctx) error {
			c.Locals("user_id", callerID)
			return h.GetWalletByID(c)
		})

		req := httptest.NewRequest("GET", "/wallets/invalid-uuid", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}
	})
}
