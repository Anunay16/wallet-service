package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
)

type mockWalletRepo struct {
	getOrCreateWalletFunc func(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	getWalletByIDFunc     func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
}

func (m *mockWalletRepo) GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	if m.getOrCreateWalletFunc != nil {
		return m.getOrCreateWalletFunc(ctx, userID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockWalletRepo) GetWalletByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	if m.getWalletByIDFunc != nil {
		return m.getWalletByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func TestNewWalletService(t *testing.T) {
	t.Run("Positive: valid repositories dependency", func(t *testing.T) {
		svc := NewWalletService(&mockWalletRepo{}, &mockUserRepo{})
		if svc == nil || svc.walletRepo == nil {
			t.Errorf("expected non-nil WalletService")
		}
	})

	t.Run("Negative 1: nil walletRepo dependency", func(t *testing.T) {
		svc := NewWalletService(nil, &mockUserRepo{})
		if svc == nil {
			t.Errorf("expected non-nil struct instance")
		}
		if svc.walletRepo != nil {
			t.Errorf("expected nil walletRepo")
		}
	})

	t.Run("Negative 2: nil userRepo dependency", func(t *testing.T) {
		svc := NewWalletService(&mockWalletRepo{}, nil)
		if svc == nil {
			t.Errorf("expected non-nil struct instance")
		}
		if svc.userRepo != nil {
			t.Errorf("expected nil userRepo")
		}
	})
}

func TestWalletService_GetOrCreateWallet(t *testing.T) {
	userID := uuid.New()
	walletID := uuid.New()

	t.Run("Positive: successful get or create wallet", func(t *testing.T) {
		walletRepo := &mockWalletRepo{
			getOrCreateWalletFunc: func(ctx context.Context, uid uuid.UUID) (*domain.Wallet, error) {
				return &domain.Wallet{
					ID:        walletID,
					UserID:    uid,
					Balance:   1000000,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		}
		userRepo := &mockUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: id, Username: "alice"}, nil
			},
		}
		svc := NewWalletService(walletRepo, userRepo)
		resp, err := svc.GetOrCreateWallet(context.Background(), userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Username != "alice" || resp.Balance != 1000000 {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("Negative 1: walletRepo returns error", func(t *testing.T) {
		walletRepo := &mockWalletRepo{
			getOrCreateWalletFunc: func(ctx context.Context, uid uuid.UUID) (*domain.Wallet, error) {
				return nil, errors.New("db error")
			},
		}
		svc := NewWalletService(walletRepo, &mockUserRepo{})
		_, err := svc.GetOrCreateWallet(context.Background(), userID)
		if err == nil {
			t.Errorf("expected error from walletRepo")
		}
	})

	t.Run("Negative 2: userRepo fails to fetch user", func(t *testing.T) {
		walletRepo := &mockWalletRepo{
			getOrCreateWalletFunc: func(ctx context.Context, uid uuid.UUID) (*domain.Wallet, error) {
				return &domain.Wallet{ID: walletID, UserID: uid}, nil
			},
		}
		userRepo := &mockUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return nil, domain.ErrUserNotFound
			},
		}
		svc := NewWalletService(walletRepo, userRepo)
		_, err := svc.GetOrCreateWallet(context.Background(), userID)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got: %v", err)
		}
	})
}

func TestWalletService_GetWalletByID(t *testing.T) {
	callerID := uuid.New()
	walletID := uuid.New()

	t.Run("Positive: caller owns the wallet", func(t *testing.T) {
		walletRepo := &mockWalletRepo{
			getWalletByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
				return &domain.Wallet{
					ID:        id,
					UserID:    callerID,
					Balance:   50000,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		}
		userRepo := &mockUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return &domain.User{ID: id, Username: "alice"}, nil
			},
		}
		svc := NewWalletService(walletRepo, userRepo)
		resp, err := svc.GetWalletByID(context.Background(), walletID, callerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ID != walletID || resp.Balance != 50000 {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("Negative 1: caller does not own the wallet (forbidden)", func(t *testing.T) {
		otherUserID := uuid.New()
		walletRepo := &mockWalletRepo{
			getWalletByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
				return &domain.Wallet{
					ID:     id,
					UserID: otherUserID,
				}, nil
			},
		}
		svc := NewWalletService(walletRepo, &mockUserRepo{})
		_, err := svc.GetWalletByID(context.Background(), walletID, callerID)
		if !errors.Is(err, domain.ErrForbiddenWalletAccess) {
			t.Errorf("expected ErrForbiddenWalletAccess, got: %v", err)
		}
	})

	t.Run("Negative 2: wallet not found error", func(t *testing.T) {
		walletRepo := &mockWalletRepo{
			getWalletByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
				return nil, domain.ErrWalletNotFound
			},
		}
		svc := NewWalletService(walletRepo, &mockUserRepo{})
		_, err := svc.GetWalletByID(context.Background(), walletID, callerID)
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})
}
