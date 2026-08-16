package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNewUserRepository(t *testing.T) {
	t.Run("Positive: valid DB dependency", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)
		if repo == nil || repo.db == nil {
			t.Errorf("expected non-nil UserRepository")
		}
	})

	t.Run("Negative 1: nil DB dependency", func(t *testing.T) {
		repo := NewUserRepository(nil)
		if repo == nil {
			t.Fatalf("expected struct instance even with nil dependency")
		}
		if repo.db != nil {
			t.Errorf("expected nil db")
		}
	})

	t.Run("Negative 2: uninitialized DB pointer", func(t *testing.T) {
		var db *gorm.DB
		repo := NewUserRepository(db)
		if repo == nil {
			t.Errorf("expected non-nil UserRepository")
		}
	})
}

func TestUserRepository_CreateUser(t *testing.T) {
	t.Run("Positive: successfully create new user", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		u := &domain.User{
			Username:     "alice",
			Email:        "alice@example.com",
			PasswordHash: "hashedpassword",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		err := repo.CreateUser(context.Background(), u)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u.ID == uuid.Nil {
			t.Errorf("expected generated UUID, got Nil")
		}
	})

	t.Run("Negative 1: duplicate username error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		u1 := &domain.User{
			ID:           uuid.New(),
			Username:     "alice",
			Email:        "alice1@example.com",
			PasswordHash: "hash1",
		}
		_ = repo.CreateUser(context.Background(), u1)

		u2 := &domain.User{
			ID:           uuid.New(),
			Username:     "alice", // Duplicate username
			Email:        "alice2@example.com",
			PasswordHash: "hash2",
		}
		err := repo.CreateUser(context.Background(), u2)
		if !errors.Is(err, domain.ErrUserAlreadyExists) && err == nil {
			t.Errorf("expected duplicate username error, got: %v", err)
		}
	})

	t.Run("Negative 2: duplicate email error", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		u1 := &domain.User{
			ID:           uuid.New(),
			Username:     "user1",
			Email:        "same@example.com",
			PasswordHash: "hash1",
		}
		_ = repo.CreateUser(context.Background(), u1)

		u2 := &domain.User{
			ID:           uuid.New(),
			Username:     "user2",
			Email:        "same@example.com", // Duplicate email
			PasswordHash: "hash2",
		}
		err := repo.CreateUser(context.Background(), u2)
		if err == nil {
			t.Errorf("expected error on duplicate email, got nil")
		}
	})
}

func TestUserRepository_GetUserByUsername(t *testing.T) {
	t.Run("Positive: successfully get user by username", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		u := &domain.User{
			ID:           uuid.New(),
			Username:     "bob",
			Email:        "bob@example.com",
			PasswordHash: "hash",
		}
		_ = repo.CreateUser(context.Background(), u)

		found, err := repo.GetUserByUsername(context.Background(), "bob")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.Username != "bob" || found.ID != u.ID {
			t.Errorf("unexpected user record: %+v", found)
		}
	})

	t.Run("Negative 1: user not found by username", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		_, err := repo.GetUserByUsername(context.Background(), "nonexistent")
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: empty username lookup", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		_, err := repo.GetUserByUsername(context.Background(), "")
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got: %v", err)
		}
	})
}

func TestUserRepository_GetUserByID(t *testing.T) {
	t.Run("Positive: successfully get user by ID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		uID := uuid.New()
		u := &domain.User{
			ID:           uID,
			Username:     "charlie",
			Email:        "charlie@example.com",
			PasswordHash: "hash",
		}
		_ = repo.CreateUser(context.Background(), u)

		found, err := repo.GetUserByID(context.Background(), uID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.ID != uID || found.Username != "charlie" {
			t.Errorf("unexpected user record: %+v", found)
		}
	})

	t.Run("Negative 1: user not found by non-existent UUID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		_, err := repo.GetUserByID(context.Background(), uuid.New())
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got: %v", err)
		}
	})

	t.Run("Negative 2: lookup with nil UUID", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewUserRepository(db)

		_, err := repo.GetUserByID(context.Background(), uuid.Nil)
		if !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("expected ErrUserNotFound, got: %v", err)
		}
	})
}
