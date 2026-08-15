package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Username     string    `json:"username" gorm:"uniqueIndex;not null"`
	Email        string    `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Wallet struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;uniqueIndex;not null"`
	Balance   int64     `json:"balance"` // In paise (BIGINT)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
	TransferStatusDeclined  TransferStatus = "declined"
)

type Transfer struct {
	ID             uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	FromWalletID   uuid.UUID      `json:"from_wallet_id" gorm:"type:uuid;not null"`
	ToWalletID     uuid.UUID      `json:"to_wallet_id" gorm:"type:uuid;not null"`
	Amount         int64          `json:"amount" gorm:"not null"` // In paise
	Status         TransferStatus `json:"status" gorm:"not null"`
	IdempotencyKey string         `json:"idempotency_key" gorm:"not null"`
	InitiatedBy    uuid.UUID      `json:"initiated_by" gorm:"type:uuid;not null"`
	FailureReason  *string        `json:"failure_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type IdempotencyRecord struct {
	IdempotencyKey string          `json:"idempotency_key" gorm:"primaryKey"`
	UserID         uuid.UUID       `json:"user_id" gorm:"type:uuid;primaryKey"`
	RequestHash    string          `json:"request_hash" gorm:"not null"`
	TransferID     *uuid.UUID      `json:"transfer_id,omitempty" gorm:"type:uuid"`
	ResponseStatus int             `json:"response_status" gorm:"not null"`
	ResponseBody   json.RawMessage `json:"response_body"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

func (IdempotencyRecord) TableName() string {
	return "idempotency_keys"
}
