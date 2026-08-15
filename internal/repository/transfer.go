package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TransferRepository struct {
	db *gorm.DB
}

func NewTransferRepository(db *gorm.DB) *TransferRepository {
	return &TransferRepository{db: db}
}

func (r *TransferRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
	var t domain.Transfer
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTransferNotFound
		}
		return nil, fmt.Errorf("failed to get transfer by id: %w", err)
	}
	return &t, nil
}

func (r *TransferRepository) ExecuteAtomicTransfer(
	ctx context.Context,
	req domain.TransferRequest,
	userID uuid.UUID,
	reqHash string,
) (*domain.Transfer, *domain.IdempotencyRecord, error) {
	var transferResult domain.Transfer
	var idemResult domain.IdempotencyRecord
	var errResult error

	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Deadlock prevention: sort lock targets in ascending UUID string order
		firstID, secondID := req.From, req.To
		if strings.Compare(firstID.String(), secondID.String()) > 0 {
			firstID, secondID = secondID, firstID
		}

		var lockedWallets []domain.Wallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []uuid.UUID{firstID, secondID}).
			Order("id ASC").
			Find(&lockedWallets).Error
		if err != nil {
			return fmt.Errorf("failed to acquire wallet row locks: %w", err)
		}

		wallets := make(map[uuid.UUID]domain.Wallet)
		for _, w := range lockedWallets {
			wallets[w.ID] = w
		}

		fromWallet, fromExists := wallets[req.From]
		_, toExists := wallets[req.To]
		if !fromExists || !toExists {
			return domain.ErrWalletNotFound
		}

		if fromWallet.UserID != userID {
			return domain.ErrForbiddenWalletAccess
		}

		// Overdraft check
		if fromWallet.Balance < req.Amount {
			reason := "insufficient_funds"
			transfer := domain.Transfer{
				ID:             uuid.New(),
				FromWalletID:   req.From,
				ToWalletID:     req.To,
				Amount:         req.Amount,
				Status:         domain.TransferStatusDeclined,
				IdempotencyKey: req.IdempotencyKey,
				InitiatedBy:    userID,
				FailureReason:  &reason,
			}
			if err := tx.Create(&transfer).Error; err != nil {
				return fmt.Errorf("failed to insert declined transfer: %w", err)
			}

			declinedStatus := "declined"
			respDTO := domain.ErrorResponse{
				Error:         reason,
				TransferID:    &transfer.ID,
				Status:        &declinedStatus,
				FailureReason: &reason,
			}
			respBytes, _ := json.Marshal(respDTO)

			idemRecord := domain.IdempotencyRecord{
				IdempotencyKey: req.IdempotencyKey,
				UserID:         userID,
				RequestHash:    reqHash,
				TransferID:     &transfer.ID,
				ResponseStatus: 402,
				ResponseBody:   respBytes,
			}
			if err := tx.Create(&idemRecord).Error; err != nil {
				return fmt.Errorf("failed to record idempotency key for declined transfer: %w", err)
			}

			transferResult = transfer
			idemResult = idemRecord
			errResult = domain.ErrInsufficientFunds
			return nil // Commit transaction with declined state
		}

		// Sufficient funds: Debit sender
		res := tx.Model(&domain.Wallet{}).
			Where("id = ? AND balance >= ?", req.From, req.Amount).
			Update("balance", gorm.Expr("balance - ?", req.Amount))
		if res.Error != nil {
			return fmt.Errorf("failed to debit sender wallet: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return domain.ErrInsufficientFunds
		}

		// Credit receiver
		if err := tx.Model(&domain.Wallet{}).
			Where("id = ?", req.To).
			Update("balance", gorm.Expr("balance + ?", req.Amount)).Error; err != nil {
			return fmt.Errorf("failed to credit receiver wallet: %w", err)
		}

		// Record completed transfer
		transfer := domain.Transfer{
			ID:             uuid.New(),
			FromWalletID:   req.From,
			ToWalletID:     req.To,
			Amount:         req.Amount,
			Status:         domain.TransferStatusCompleted,
			IdempotencyKey: req.IdempotencyKey,
			InitiatedBy:    userID,
		}
		if err := tx.Create(&transfer).Error; err != nil {
			return fmt.Errorf("failed to insert completed transfer record: %w", err)
		}

		respDTO := domain.TransferResponse{
			ID:            transfer.ID,
			FromWalletID:  transfer.FromWalletID,
			ToWalletID:    transfer.ToWalletID,
			Amount:        transfer.Amount,
			Status:        transfer.Status,
			FailureReason: transfer.FailureReason,
			CreatedAt:     transfer.CreatedAt,
			UpdatedAt:     transfer.UpdatedAt,
		}
		respBytes, _ := json.Marshal(respDTO)

		idemRecord := domain.IdempotencyRecord{
			IdempotencyKey: req.IdempotencyKey,
			UserID:         userID,
			RequestHash:    reqHash,
			TransferID:     &transfer.ID,
			ResponseStatus: 201,
			ResponseBody:   respBytes,
		}
		if err := tx.Create(&idemRecord).Error; err != nil {
			return fmt.Errorf("failed to record idempotency key for completed transfer: %w", err)
		}

		transferResult = transfer
		idemResult = idemRecord
		return nil
	})

	if txErr != nil {
		return nil, nil, txErr
	}

	return &transferResult, &idemResult, errResult
}
