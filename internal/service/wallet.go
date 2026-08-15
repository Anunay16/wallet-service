package service

import (
	"context"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
)

type walletRepo interface {
	GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error)
	GetWalletByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
}

type WalletService struct {
	walletRepo walletRepo
	userRepo   userRepo
}

func NewWalletService(walletRepo walletRepo, userRepo userRepo) *WalletService {
	return &WalletService{
		walletRepo: walletRepo,
		userRepo:   userRepo,
	}
}

func (s *WalletService) GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.WalletResponse, error) {
	w, err := s.walletRepo.GetOrCreateWallet(ctx, userID)
	if err != nil {
		return nil, err
	}

	user, err := s.userRepo.GetUserByID(ctx, w.UserID)
	if err != nil {
		return nil, err
	}

	return &domain.WalletResponse{
		ID:        w.ID,
		Username:  user.Username,
		Balance:   w.Balance,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}, nil
}

func (s *WalletService) GetWalletByID(ctx context.Context, walletID uuid.UUID, callerUserID uuid.UUID) (*domain.WalletResponse, error) {
	w, err := s.walletRepo.GetWalletByID(ctx, walletID)
	if err != nil {
		return nil, err
	}

	if w.UserID != callerUserID {
		return nil, domain.ErrForbiddenWalletAccess
	}

	user, err := s.userRepo.GetUserByID(ctx, w.UserID)
	if err != nil {
		return nil, err
	}

	return &domain.WalletResponse{
		ID:        w.ID,
		Username:  user.Username,
		Balance:   w.Balance,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}, nil
}
