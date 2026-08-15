package domain

import "errors"

var (
	ErrUserAlreadyExists     = errors.New("username or email already registered")
	ErrUserNotFound          = errors.New("user not found")
	ErrInvalidCredentials    = errors.New("invalid username or password")
	ErrWalletNotFound        = errors.New("wallet not found")
	ErrForbiddenWalletAccess = errors.New("forbidden: caller does not own this wallet")
	ErrInsufficientFunds     = errors.New("insufficient_funds")
	ErrSameWalletTransfer    = errors.New("cannot transfer funds to the same wallet")
	ErrInvalidAmount         = errors.New("amount must be greater than 0")
	ErrEmptyIdempotencyKey   = errors.New("idempotency_key is required")
	ErrIdempotencyConflict   = errors.New("idempotency_key_conflict")
	ErrTransferNotFound      = errors.New("transfer record not found")
)
