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

func TestNewTransferRepository(t *testing.T) {
	t.Run("Positive: valid DB dependency", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)
		if repo == nil || repo.db == nil {
			t.Errorf("expected non-nil TransferRepository")
		}
	})

	t.Run("Negative 1: nil DB dependency", func(t *testing.T) {
		repo := NewTransferRepository(nil)
		if repo == nil {
			t.Errorf("expected struct instance even with nil dependency")
		}
		if repo.db != nil {
			t.Errorf("expected nil db")
		}
	})

	t.Run("Negative 2: uninitialized DB pointer", func(t *testing.T) {
		var db *gorm.DB
		repo := NewTransferRepository(db)
		if repo == nil {
			t.Errorf("expected non-nil TransferRepository")
		}
	})
}

func TestTransferRepository_GetByID(t *testing.T) {
	t.Run("Positive: successfully get transfer by ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		tID := uuid.New()
		tr := &domain.Transfer{
			ID:             tID,
			FromWalletID:   uuid.New(),
			ToWalletID:     uuid.New(),
			Amount:         5000,
			Status:         domain.TransferStatusCompleted,
			IdempotencyKey: "key-123",
			InitiatedBy:    uuid.New(),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		db.Create(tr)

		found, err := repo.GetByID(context.Background(), tID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.ID != tID || found.Amount != 5000 {
			t.Errorf("unexpected transfer: %+v", found)
		}
	})

	t.Run("Negative 1: transfer not found by ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		_, err := repo.GetByID(context.Background(), uuid.New())
		if !errors.Is(err, domain.ErrTransferNotFound) {
			t.Errorf("expected ErrTransferNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: transfer not found by nil UUID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		_, err := repo.GetByID(context.Background(), uuid.Nil)
		if !errors.Is(err, domain.ErrTransferNotFound) {
			t.Errorf("expected ErrTransferNotFound, got: %v", err)
		}
	})
}

func TestTransferRepository_ExecuteAtomicTransfer(t *testing.T) {
	t.Run("Positive: successful atomic transfer with sufficient balance", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		fromUserID := uuid.New()
		toUserID := uuid.New()
		fromWallet := &domain.Wallet{ID: uuid.New(), UserID: fromUserID, Balance: 10000}
		toWallet := &domain.Wallet{ID: uuid.New(), UserID: toUserID, Balance: 2000}
		db.Create(fromWallet)
		db.Create(toWallet)

		req := domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         3000,
			IdempotencyKey: "idem-key-1",
		}

		tr, idem, err := repo.ExecuteAtomicTransfer(context.Background(), req, fromUserID, toUserID, fromUserID, "hash-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.Status != domain.TransferStatusCompleted {
			t.Errorf("expected transfer status completed, got: %s", tr.Status)
		}
		if idem.ResponseStatus != 201 {
			t.Errorf("expected response status 201, got: %d", idem.ResponseStatus)
		}

		// Verify updated balances in DB
		var updatedFrom, updatedTo domain.Wallet
		db.First(&updatedFrom, "id = ?", fromWallet.ID)
		db.First(&updatedTo, "id = ?", toWallet.ID)
		if updatedFrom.Balance != 7000 {
			t.Errorf("expected sender balance 7000, got: %d", updatedFrom.Balance)
		}
		if updatedTo.Balance != 5000 {
			t.Errorf("expected receiver balance 5000, got: %d", updatedTo.Balance)
		}
	})

	t.Run("Negative 1: insufficient funds causes declined transfer and idempotency record", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		fromUserID := uuid.New()
		toUserID := uuid.New()
		fromWallet := &domain.Wallet{ID: uuid.New(), UserID: fromUserID, Balance: 500}
		toWallet := &domain.Wallet{ID: uuid.New(), UserID: toUserID, Balance: 2000}
		db.Create(fromWallet)
		db.Create(toWallet)

		req := domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         3000,
			IdempotencyKey: "idem-key-2",
		}

		tr, idem, err := repo.ExecuteAtomicTransfer(context.Background(), req, fromUserID, toUserID, fromUserID, "hash-2")
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got: %v", err)
		}
		if tr == nil || tr.Status != domain.TransferStatusDeclined {
			t.Errorf("expected declined transfer struct")
		}
		if idem == nil || idem.ResponseStatus != 402 {
			t.Errorf("expected 402 response status in idempotency record")
		}
	})

	t.Run("Negative 2: non-existent wallet error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewTransferRepository(db)

		fromUserID := uuid.New()
		toUserID := uuid.New()

		req := domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         1000,
			IdempotencyKey: "idem-key-3",
		}

		_, _, err := repo.ExecuteAtomicTransfer(context.Background(), req, fromUserID, toUserID, fromUserID, "hash-3")
		if !errors.Is(err, domain.ErrWalletNotFound) {
			t.Errorf("expected ErrWalletNotFound, got: %v", err)
		}
	})
}
