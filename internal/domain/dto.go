package domain

import (
	"time"

	"github.com/google/uuid"
)

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
	Username  string    `json:"username"`
	Balance   int64     `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TransferRequest struct {
	From           string `json:"from,omitempty"`
	To             string `json:"to"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
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
