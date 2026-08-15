package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
)

type transferRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Transfer, error)
	ExecuteAtomicTransfer(ctx context.Context, req domain.TransferRequest, userID uuid.UUID, reqHash string) (*domain.Transfer, *domain.IdempotencyRecord, error)
}

type transferWalletRepo interface {
	GetWalletByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
}

type idempotencyRepo interface {
	Get(ctx context.Context, key string, userID uuid.UUID) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, record *domain.IdempotencyRecord) error
}

type TransferService struct {
	transferRepo    transferRepo
	walletRepo      transferWalletRepo
	idempotencyRepo idempotencyRepo
}

func NewTransferService(
	transferRepo transferRepo,
	walletRepo transferWalletRepo,
	idempotencyRepo idempotencyRepo,
) *TransferService {
	return &TransferService{
		transferRepo:    transferRepo,
		walletRepo:      walletRepo,
		idempotencyRepo: idempotencyRepo,
	}
}

func (s *TransferService) InitiateTransfer(
	ctx context.Context,
	req domain.TransferRequest,
	callerUserID uuid.UUID,
) (*domain.IdempotencyRecord, error) {
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)

	if req.From == uuid.Nil {
		req.From = callerUserID
	}

	if req.Amount <= 0 {
		return nil, domain.ErrInvalidAmount
	}
	if req.From == req.To {
		return nil, domain.ErrSameWalletTransfer
	}
	if req.IdempotencyKey == "" {
		return nil, domain.ErrEmptyIdempotencyKey
	}

	// Ensure caller is initiating their own transfer
	if req.From != callerUserID {
		return nil, domain.ErrForbiddenWalletAccess
	}

	// Canonical request hash
	reqHash := computeRequestHash(req.From, req.To, req.Amount)

	// Idempotency lookup
	existing, err := s.idempotencyRepo.Get(ctx, req.IdempotencyKey, callerUserID)
	if err != nil {
		return nil, fmt.Errorf("idempotency check failed: %w", err)
	}

	if existing != nil {
		if existing.RequestHash != reqHash {
			return nil, domain.ErrIdempotencyConflict
		}
		// Return cached idempotency record (200 OK replay)
		return existing, nil
	}

	// Execute atomic transfer (locks rows in ascending order, updates balances, saves transfer + idempotency)
	_, idemRecord, err := s.transferRepo.ExecuteAtomicTransfer(ctx, req, callerUserID, reqHash)
	if err != nil && idemRecord == nil {
		return nil, err
	}

	return idemRecord, err
}

func (s *TransferService) GetTransferByID(
	ctx context.Context,
	transferID uuid.UUID,
	callerUserID uuid.UUID,
) (*domain.Transfer, error) {
	t, err := s.transferRepo.GetByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	// Check access rights: initiated_by or owner of source/destination wallet
	if t.InitiatedBy == callerUserID {
		return t, nil
	}

	fromWallet, err := s.walletRepo.GetWalletByID(ctx, t.FromWalletID)
	if err == nil && fromWallet.UserID == callerUserID {
		return t, nil
	}

	toWallet, err := s.walletRepo.GetWalletByID(ctx, t.ToWalletID)
	if err == nil && toWallet.UserID == callerUserID {
		return t, nil
	}

	return nil, domain.ErrForbiddenWalletAccess
}

func computeRequestHash(from, to uuid.UUID, amount int64) string {
	raw := fmt.Sprintf("%s:%s:%d", from.String(), to.String(), amount)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
