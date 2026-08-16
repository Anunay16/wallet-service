package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNewIdempotencyRepository(t *testing.T) {
	t.Run("Positive: valid DB dependency", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)
		if repo == nil || repo.db == nil {
			t.Errorf("expected non-nil IdempotencyRepository")
		}
	})

	t.Run("Negative 1: nil DB dependency", func(t *testing.T) {
		repo := NewIdempotencyRepository(nil)
		if repo == nil {
			t.Fatalf("expected struct instance even with nil dependency")
		}
		if repo.db != nil {
			t.Errorf("expected nil db")
		}
	})

	t.Run("Negative 2: uninitialized DB pointer", func(t *testing.T) {
		var db *gorm.DB
		repo := NewIdempotencyRepository(db)
		if repo == nil {
			t.Errorf("expected non-nil IdempotencyRepository")
		}
	})
}

func TestIdempotencyRepository_Get(t *testing.T) {
	t.Run("Positive: successfully get idempotency record", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)

		uID := uuid.New()
		rec := &domain.IdempotencyRecord{
			IdempotencyKey: "key-1",
			UserID:         uID,
			RequestHash:    "hash-1",
			ResponseStatus: 201,
			ResponseBody:   json.RawMessage(`{"status":"ok"}`),
			CreatedAt:      time.Now(),
		}
		_ = repo.Save(context.Background(), rec)

		found, err := repo.Get(context.Background(), "key-1", uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found == nil || found.RequestHash != "hash-1" {
			t.Errorf("unexpected record: %+v", found)
		}
	})

	t.Run("Negative 1: non-existent key returns nil record and nil error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)

		found, err := repo.Get(context.Background(), "non-existent-key", uuid.New())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found != nil {
			t.Errorf("expected nil record for non-existent key, got: %+v", found)
		}
	})

	t.Run("Negative 2: key belonging to different user returns nil record", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)

		uID1 := uuid.New()
		uID2 := uuid.New()
		rec := &domain.IdempotencyRecord{
			IdempotencyKey: "key-2",
			UserID:         uID1,
			RequestHash:    "hash-2",
			ResponseStatus: 200,
			ResponseBody:   json.RawMessage(`{"status":"ok"}`),
		}
		_ = repo.Save(context.Background(), rec)

		found, err := repo.Get(context.Background(), "key-2", uID2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found != nil {
			t.Errorf("expected nil record when querying with different user ID, got: %+v", found)
		}
	})
}

func TestIdempotencyRepository_Save(t *testing.T) {
	t.Run("Positive: successfully save new idempotency record", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)

		rec := &domain.IdempotencyRecord{
			IdempotencyKey: "save-key-1",
			UserID:         uuid.New(),
			RequestHash:    "hash-save-1",
			ResponseStatus: 201,
			ResponseBody:   json.RawMessage(`{"status":"created"}`),
		}
		err := repo.Save(context.Background(), rec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Negative 1: database connection closed error", func(t *testing.T) {
		db := newTestDB(t)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
		repo := NewIdempotencyRepository(db)

		rec := &domain.IdempotencyRecord{
			IdempotencyKey: "save-key-2",
			UserID:         uuid.New(),
			RequestHash:    "hash-save-2",
		}
		err := repo.Save(context.Background(), rec)
		if err == nil {
			t.Errorf("expected error saving record on closed database connection")
		}
	})

	t.Run("Negative 2: duplicate key insertion error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewIdempotencyRepository(db)

		rec1 := &domain.IdempotencyRecord{
			IdempotencyKey: "duplicate-key",
			UserID:         uuid.New(),
			RequestHash:    "hash-1",
		}
		_ = repo.Save(context.Background(), rec1)

		rec2 := &domain.IdempotencyRecord{
			IdempotencyKey: "duplicate-key",
			UserID:         rec1.UserID,
			RequestHash:    "hash-2",
		}
		err := repo.Save(context.Background(), rec2)
		if err == nil {
			t.Errorf("expected error on duplicate key insertion")
		}
	})
}
