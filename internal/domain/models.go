package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Wallet struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Balance   int64     `json:"balance"` // In paise (BIGINT)
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
	TransferStatusDeclined TransferStatus = "declined"
)

type Transfer struct {
	ID             uuid.UUID      `json:"id"`
	FromWalletID   uuid.UUID      `json:"from_wallet_id"`
	ToWalletID     uuid.UUID      `json:"to_wallet_id"`
	Amount         int64          `json:"amount"` // In paise
	Status         TransferStatus `json:"status"`
	IdempotencyKey string         `json:"idempotency_key"`
	InitiatedBy    uuid.UUID      `json:"initiated_by"`
	FailureReason  *string        `json:"failure_reason,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type IdempotencyRecord struct {
	IdempotencyKey string          `json:"idempotency_key"`
	UserID         uuid.UUID       `json:"user_id"`
	RequestHash    string          `json:"request_hash"`
	TransferID     *uuid.UUID      `json:"transfer_id,omitempty"`
	ResponseStatus int             `json:"response_status"`
	ResponseBody   json.RawMessage `json:"response_body"`
	CreatedAt      time.Time       `json:"created_at"`
	ExpiresAt      time.Time       `json:"expires_at"`
}

func (IdempotencyRecord) TableName() string {
	return "idempotency_keys"
}

// Request & Response DTOs

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type UserResponse struct {
	ID       uuid.UUID `json:"id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
}

type WalletResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Balance   int64     `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransferRequest struct {
	From           uuid.UUID `json:"from"`
	To             uuid.UUID `json:"to"`
	Amount         int64     `json:"amount"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type TransferResponse struct {
	ID            uuid.UUID      `json:"id"`
	FromWalletID  uuid.UUID      `json:"from_wallet_id"`
	ToWalletID    uuid.UUID      `json:"to_wallet_id"`
	Amount        int64          `json:"amount"`
	Status        TransferStatus `json:"status"`
	FailureReason *string        `json:"failure_reason,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type ErrorResponse struct {
	Error         string     `json:"error"`
	TransferID    *uuid.UUID `json:"transfer_id,omitempty"`
	Status        *string    `json:"status,omitempty"`
	FailureReason *string    `json:"failure_reason,omitempty"`
}
