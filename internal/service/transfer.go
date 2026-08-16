package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/anunay/wallet-service/internal/metrics"
	"github.com/anunay/wallet-service/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type transferRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Transfer, error)
	ExecuteAtomicTransfer(ctx context.Context, req domain.TransferRequest, fromUserID, toUserID, userID uuid.UUID, reqHash string) (*domain.Transfer, *domain.IdempotencyRecord, error)
}

type transferUserRepo interface {
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
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
	userRepo        transferUserRepo
	walletRepo      transferWalletRepo
	idempotencyRepo idempotencyRepo
	log             *zap.Logger
}

type TransferServiceOption func(*TransferService)

func WithLogger(log *zap.Logger) TransferServiceOption {
	return func(s *TransferService) {
		if log != nil {
			s.log = log
		}
	}
}

func NewTransferService(
	transferRepo transferRepo,
	userRepo transferUserRepo,
	walletRepo transferWalletRepo,
	idempotencyRepo idempotencyRepo,
	opts ...TransferServiceOption,
) *TransferService {
	s := &TransferService{
		transferRepo:    transferRepo,
		userRepo:        userRepo,
		walletRepo:      walletRepo,
		idempotencyRepo: idempotencyRepo,
		log:             zap.NewNop(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *TransferService) InitiateTransfer(
	ctx context.Context,
	req domain.TransferRequest,
	callerUserID uuid.UUID,
) (*domain.IdempotencyRecord, error) {
	cid := middleware.CorrelationIDFromContext(ctx)
	req.From = strings.TrimSpace(req.From)
	req.To = strings.TrimSpace(req.To)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)

	// Fetch caller user to get their username
	callerUser, err := s.userRepo.GetUserByID(ctx, callerUserID)
	if err != nil {
		s.log.Error("InitiateTransfer failed to fetch caller user",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("caller_user_id", callerUserID.String()),
		)
		return nil, fmt.Errorf("failed to fetch caller user: %w", err)
	}

	if req.From == "" {
		req.From = callerUser.Username
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
	if req.From != callerUser.Username {
		return nil, domain.ErrForbiddenWalletAccess
	}

	// Resolve destination user by username
	toUser, err := s.userRepo.GetUserByUsername(ctx, req.To)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrWalletNotFound
		}
		s.log.Error("InitiateTransfer failed to lookup destination user",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("to_username", req.To),
		)
		return nil, fmt.Errorf("failed to lookup destination user: %w", err)
	}

	// Canonical request hash using lowercased usernames
	reqHash := computeRequestHash(req.From, req.To, req.Amount)

	// Idempotency lookup
	existing, err := s.idempotencyRepo.Get(ctx, req.IdempotencyKey, callerUserID)
	if err != nil {
		s.log.Error("InitiateTransfer idempotency lookup failed",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("idempotency_key", req.IdempotencyKey),
		)
		return nil, fmt.Errorf("idempotency check failed: %w", err)
	}

	if existing != nil {
		if existing.RequestHash != reqHash {
			return nil, domain.ErrIdempotencyConflict
		}
		// Record metric and emit structured domain event
		metrics.GetCollector().IncIdempotentReplays()
		s.log.Info("Domain Event: idempotent_replay_hit",
			zap.String("event", "idempotent_replay_hit"),
			zap.String("correlation_id", cid),
			zap.String("idempotency_key", req.IdempotencyKey),
			zap.String("user_id", callerUserID.String()),
			zap.Int("response_status", existing.ResponseStatus),
		)
		// Return cached idempotency record (200 OK replay)
		return existing, nil
	}

	// Domain event: transfer_created
	metrics.GetCollector().IncTransfersCreated()
	s.log.Info("Domain Event: transfer_created",
		zap.String("event", "transfer_created"),
		zap.String("correlation_id", cid),
		zap.String("from_user", req.From),
		zap.String("to_user", req.To),
		zap.Int64("amount", req.Amount),
		zap.String("idempotency_key", req.IdempotencyKey),
	)

	// Execute atomic transfer (locks rows in ascending order, updates balances, saves transfer + idempotency)
	transferRecord, idemRecord, err := s.transferRepo.ExecuteAtomicTransfer(ctx, req, callerUser.ID, toUser.ID, callerUserID, reqHash)

	if errors.Is(err, domain.ErrInsufficientFunds) {
		metrics.GetCollector().IncDeclinedInsufficientFunds()
		s.log.Warn("Domain Event: declined",
			zap.String("event", "declined"),
			zap.String("correlation_id", cid),
			zap.String("reason", "insufficient_funds"),
			zap.String("from_user", req.From),
			zap.String("to_user", req.To),
			zap.Int64("amount", req.Amount),
			zap.String("idempotency_key", req.IdempotencyKey),
		)
	} else if err != nil {
		s.log.Error("InitiateTransfer atomic execution failed",
			zap.Error(err),
			zap.String("correlation_id", cid),
			zap.String("from_user", req.From),
			zap.String("to_user", req.To),
		)
	} else if idemRecord != nil {
		// Log debited & credited domain events
		s.log.Info("Domain Event: debited",
			zap.String("event", "debited"),
			zap.String("correlation_id", cid),
			zap.String("from_user", req.From),
			zap.Int64("amount", req.Amount),
		)
		s.log.Info("Domain Event: credited",
			zap.String("event", "credited"),
			zap.String("correlation_id", cid),
			zap.String("to_user", req.To),
			zap.Int64("amount", req.Amount),
		)
	}

	if err != nil && idemRecord == nil {
		return nil, err
	}

	_ = transferRecord
	return idemRecord, err
}

func (s *TransferService) GetTransferByID(
	ctx context.Context,
	transferID uuid.UUID,
	callerUserID uuid.UUID,
) (*domain.Transfer, error) {
	t, err := s.transferRepo.GetByID(ctx, transferID)
	if err != nil {
		if !errors.Is(err, domain.ErrTransferNotFound) {
			cid := middleware.CorrelationIDFromContext(ctx)
			s.log.Error("GetTransferByID repository call failed",
				zap.Error(err),
				zap.String("correlation_id", cid),
				zap.String("transfer_id", transferID.String()),
			)
		}
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

func computeRequestHash(from, to string, amount int64) string {
	raw := fmt.Sprintf("%s:%s:%d", strings.ToLower(from), strings.ToLower(to), amount)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
