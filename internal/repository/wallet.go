package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) GetOrCreateWallet(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	var w domain.Wallet
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&w).Error
	if err == nil {
		return &w, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to query wallet: %w", err)
	}

	newWallet := domain.Wallet{
		ID:      uuid.New(),
		UserID:  userID,
		Balance: 1000000, // 1,000,000 paise = ₹10,000 seed balance
	}

	err = r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"updated_at": gorm.Expr("wallets.updated_at"),
			}),
		}).
		Create(&newWallet).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	err = r.db.WithContext(ctx).Where("user_id = ?", userID).First(&w).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch wallet post-creation: %w", err)
	}

	return &w, nil
}

func (r *WalletRepository) GetWalletByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	var w domain.Wallet
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to get wallet by id: %w", err)
	}
	return &w, nil
}

func (r *WalletRepository) GetWalletByUserID(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	var w domain.Wallet
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&w).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrWalletNotFound
		}
		return nil, fmt.Errorf("failed to get wallet by user id: %w", err)
	}
	return &w, nil
}
