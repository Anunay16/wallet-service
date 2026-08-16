package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNewWalletRepository(t *testing.T) {
	t.Run("Positive: valid DB dependency", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)
		if repo == nil || repo.db == nil {
			t.Errorf("expected non-nil WalletRepository")
		}
	})

	t.Run("Negative 1: nil DB dependency", func(t *testing.T) {
		repo := NewWalletRepository(nil)
		if repo == nil {
			t.Fatalf("expected struct instance even with nil dependency")
		}
		if repo.db != nil {
			t.Errorf("expected nil db")
		}
	})

	t.Run("Negative 2: uninitialized DB pointer", func(t *testing.T) {
		var db *gorm.DB
		repo := NewWalletRepository(db)
		if repo == nil {
			t.Errorf("expected non-nil WalletRepository")
		}
	})
}

func TestWalletRepository_GetOrCreateWallet(t *testing.T) {
	t.Run("Positive: creates wallet when missing", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		uID := uuid.New()
		w, err := repo.GetOrCreateWallet(context.Background(), uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w == nil || w.UserID != uID {
			t.Errorf("unexpected wallet returned: %+v", w)
		}
	})

	t.Run("Positive: returns existing wallet on subsequent call", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		uID := uuid.New()
		w1, err := repo.GetOrCreateWallet(context.Background(), uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w2, err := repo.GetOrCreateWallet(context.Background(), uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w2.ID != w1.ID {
			t.Errorf("expected same wallet ID %s, got %s", w1.ID, w2.ID)
		}
	})

	t.Run("Negative 1: database connection closed", func(t *testing.T) {
		db := newTestDB(t)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		repo := NewWalletRepository(db)

		_, err := repo.GetOrCreateWallet(context.Background(), uuid.New())
		if err == nil {
			t.Errorf("expected error when DB connection is closed")
		}
	})

	t.Run("Negative 2: canceled context error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel context immediately

		_, err := repo.GetOrCreateWallet(ctx, uuid.New())
		if err == nil {
			t.Errorf("expected context canceled error")
		}
	})
}

func TestWalletRepository_GetWalletByID(t *testing.T) {
	t.Run("Positive: successfully get wallet by ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		wID := uuid.New()
		uID := uuid.New()
		w := &domain.Wallet{
			ID:        wID,
			UserID:    uID,
			Balance:   50000,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		db.Create(w)

		found, err := repo.GetWalletByID(context.Background(), wID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.ID != wID || found.Balance != 50000 {
			t.Errorf("unexpected wallet: %+v", found)
		}
	})

	t.Run("Negative 1: wallet not found by ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		_, err := repo.GetWalletByID(context.Background(), uuid.New())
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: wallet not found by nil UUID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		_, err := repo.GetWalletByID(context.Background(), uuid.Nil)
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})
}

func TestWalletRepository_GetWalletByUserID(t *testing.T) {
	t.Run("Positive: successfully get wallet by user ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		wID := uuid.New()
		uID := uuid.New()
		w := &domain.Wallet{
			ID:        wID,
			UserID:    uID,
			Balance:   75000,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		db.Create(w)

		found, err := repo.GetWalletByUserID(context.Background(), uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.UserID != uID || found.Balance != 75000 {
			t.Errorf("unexpected wallet: %+v", found)
		}
	})

	t.Run("Negative 1: wallet not found by user ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		_, err := repo.GetWalletByUserID(context.Background(), uuid.New())
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: wallet not found by nil user UUID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewWalletRepository(db)

		_, err := repo.GetWalletByUserID(context.Background(), uuid.Nil)
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})
}
