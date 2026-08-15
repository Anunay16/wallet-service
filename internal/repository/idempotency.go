package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IdempotencyRepository struct {
	db *gorm.DB
}

func NewIdempotencyRepository(db *gorm.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) Get(ctx context.Context, key string, userID uuid.UUID) (*domain.IdempotencyRecord, error) {
	var rec domain.IdempotencyRecord
	err := r.db.WithContext(ctx).Where("idempotency_key = ? AND user_id = ?", key, userID).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get idempotency key: %w", err)
	}
	return &rec, nil
}

func (r *IdempotencyRepository) Save(ctx context.Context, record *domain.IdempotencyRecord) error {
	err := r.db.WithContext(ctx).Create(record).Error
	if err != nil {
		return fmt.Errorf("failed to save idempotency key: %w", err)
	}
	return nil
}
