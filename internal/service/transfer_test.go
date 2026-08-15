package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
)

type mockTransferRepo struct {
	getByIDFunc               func(ctx context.Context, id uuid.UUID) (*domain.Transfer, error)
	executeAtomicTransferFunc func(ctx context.Context, req domain.TransferRequest, fromUserID, toUserID, userID uuid.UUID, reqHash string) (*domain.Transfer, *domain.IdempotencyRecord, error)
}

func (m *mockTransferRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTransferRepo) ExecuteAtomicTransfer(ctx context.Context, req domain.TransferRequest, fromUserID, toUserID, userID uuid.UUID, reqHash string) (*domain.Transfer, *domain.IdempotencyRecord, error) {
	if m.executeAtomicTransferFunc != nil {
		return m.executeAtomicTransferFunc(ctx, req, fromUserID, toUserID, userID, reqHash)
	}
	return nil, nil, errors.New("not implemented")
}

type mockTransferUserRepo struct {
	getUserByUsernameFunc func(ctx context.Context, username string) (*domain.User, error)
	getUserByIDFunc       func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (m *mockTransferUserRepo) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	if m.getUserByUsernameFunc != nil {
		return m.getUserByUsernameFunc(ctx, username)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTransferUserRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

type mockTransferWalletRepo struct {
	getWalletByIDFunc func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
}

func (m *mockTransferWalletRepo) GetWalletByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	if m.getWalletByIDFunc != nil {
		return m.getWalletByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

type mockIdempotencyRepo struct {
	getFunc  func(ctx context.Context, key string, userID uuid.UUID) (*domain.IdempotencyRecord, error)
	saveFunc func(ctx context.Context, record *domain.IdempotencyRecord) error
}

func (m *mockIdempotencyRepo) Get(ctx context.Context, key string, userID uuid.UUID) (*domain.IdempotencyRecord, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, key, userID)
	}
	return nil, nil
}

func (m *mockIdempotencyRepo) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, record)
	}
	return nil
}

func TestNewTransferService(t *testing.T) {
	t.Run("Positive: valid repositories dependency", func(t *testing.T) {
		svc := NewTransferService(&mockTransferRepo{}, &mockTransferUserRepo{}, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		if svc == nil || svc.transferRepo == nil {
			t.Errorf("expected non-nil TransferService")
		}
	})

	t.Run("Negative 1: nil transferRepo dependency", func(t *testing.T) {
		svc := NewTransferService(nil, &mockTransferUserRepo{}, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		if svc == nil {
			t.Errorf("expected non-nil struct instance")
		}
		if svc.transferRepo != nil {
			t.Errorf("expected nil transferRepo")
		}
	})

	t.Run("Negative 2: nil userRepo dependency", func(t *testing.T) {
		svc := NewTransferService(&mockTransferRepo{}, nil, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		if svc == nil {
			t.Errorf("expected non-nil struct instance")
		}
		if svc.userRepo != nil {
			t.Errorf("expected nil userRepo")
		}
	})
}

func TestTransferService_InitiateTransfer(t *testing.T) {
	callerID := uuid.New()
	toUserID := uuid.New()
	callerUser := &domain.User{ID: callerID, Username: "alice"}
	toUser := &domain.User{ID: toUserID, Username: "bob"}

	t.Run("Positive: successful transfer initiation", func(t *testing.T) {
		userRepo := &mockTransferUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return callerUser, nil
			},
			getUserByUsernameFunc: func(ctx context.Context, username string) (*domain.User, error) {
				return toUser, nil
			},
		}
		idempotencyRepo := &mockIdempotencyRepo{
			getFunc: func(ctx context.Context, key string, userID uuid.UUID) (*domain.IdempotencyRecord, error) {
				return nil, nil // No existing key
			},
		}
		transferRepo := &mockTransferRepo{
			executeAtomicTransferFunc: func(ctx context.Context, req domain.TransferRequest, fromUserID, toUserID, userID uuid.UUID, reqHash string) (*domain.Transfer, *domain.IdempotencyRecord, error) {
				tID := uuid.New()
				return &domain.Transfer{
						ID:     tID,
						Amount: req.Amount,
						Status: domain.TransferStatusCompleted,
					}, &domain.IdempotencyRecord{
						IdempotencyKey: req.IdempotencyKey,
						UserID:         userID,
						ResponseStatus: 201,
						ResponseBody:   json.RawMessage(`{"status":"completed"}`),
					}, nil
			},
		}

		svc := NewTransferService(transferRepo, userRepo, &mockTransferWalletRepo{}, idempotencyRepo)
		record, err := svc.InitiateTransfer(context.Background(), domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         1000,
			IdempotencyKey: "key-1",
		}, callerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if record == nil || record.ResponseStatus != 201 {
			t.Errorf("unexpected record: %+v", record)
		}
	})

	t.Run("Negative 1: invalid transfer amount (<= 0)", func(t *testing.T) {
		userRepo := &mockTransferUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return callerUser, nil
			},
		}
		svc := NewTransferService(&mockTransferRepo{}, userRepo, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		_, err := svc.InitiateTransfer(context.Background(), domain.TransferRequest{
			From:           "alice",
			To:             "bob",
			Amount:         0,
			IdempotencyKey: "key-1",
		}, callerID)
		if !errors.Is(err, domain.ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount, got: %v", err)
		}
	})

	t.Run("Negative 2: same wallet transfer (From == To)", func(t *testing.T) {
		userRepo := &mockTransferUserRepo{
			getUserByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				return callerUser, nil
			},
		}
		svc := NewTransferService(&mockTransferRepo{}, userRepo, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		_, err := svc.InitiateTransfer(context.Background(), domain.TransferRequest{
			From:           "alice",
			To:             "alice",
			Amount:         1000,
			IdempotencyKey: "key-1",
		}, callerID)
		if !errors.Is(err, domain.ErrSameWalletTransfer) {
			t.Errorf("expected ErrSameWalletTransfer, got: %v", err)
		}
	})
}

func TestTransferService_GetTransferByID(t *testing.T) {
	callerID := uuid.New()
	transferID := uuid.New()

	t.Run("Positive: caller is the initiator of transfer", func(t *testing.T) {
		transferRepo := &mockTransferRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
				return &domain.Transfer{
					ID:          id,
					InitiatedBy: callerID,
					Amount:      500,
					Status:      domain.TransferStatusCompleted,
				}, nil
			},
		}
		svc := NewTransferService(transferRepo, &mockTransferUserRepo{}, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		tr, err := svc.GetTransferByID(context.Background(), transferID, callerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.ID != transferID {
			t.Errorf("expected transfer ID %s, got: %s", transferID, tr.ID)
		}
	})

	t.Run("Negative 1: transfer not found error", func(t *testing.T) {
		transferRepo := &mockTransferRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
				return nil, domain.ErrTransferNotFound
			},
		}
		svc := NewTransferService(transferRepo, &mockTransferUserRepo{}, &mockTransferWalletRepo{}, &mockIdempotencyRepo{})
		_, err := svc.GetTransferByID(context.Background(), transferID, callerID)
		if !errors.Is(err, domain.ErrTransferNotFound) {
			t.Errorf("expected ErrTransferNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: caller is unauthorized to view transfer", func(t *testing.T) {
		otherUserID := uuid.New()
		transferRepo := &mockTransferRepo{
			getByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
				return &domain.Transfer{
					ID:           id,
					InitiatedBy:  otherUserID,
					FromWalletID: uuid.New(),
					ToWalletID:   uuid.New(),
					Amount:       500,
					Status:       domain.TransferStatusCompleted,
				}, nil
			},
		}
		walletRepo := &mockTransferWalletRepo{
			getWalletByIDFunc: func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
				return &domain.Wallet{ID: id, UserID: otherUserID}, nil
			},
		}
		svc := NewTransferService(transferRepo, &mockTransferUserRepo{}, walletRepo, &mockIdempotencyRepo{})
		_, err := svc.GetTransferByID(context.Background(), transferID, callerID)
		if !errors.Is(err, domain.ErrForbiddenWalletAccess) {
			t.Errorf("expected ErrForbiddenWalletAccess, got: %v", err)
		}
	})
}
