package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	createUserFunc        func(ctx context.Context, user *domain.User) error
	getUserByUsernameFunc func(ctx context.Context, username string) (*domain.User, error)
	getUserByIDFunc       func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (m *mockUserRepo) CreateUser(ctx context.Context, user *domain.User) error {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, user)
	}
	return errors.New("not implemented")
}

func (m *mockUserRepo) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	if m.getUserByUsernameFunc != nil {
		return m.getUserByUsernameFunc(ctx, username)
	}
	return nil, errors.New("not implemented")
}

func (m *mockUserRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	return nil, errors.New("not implemented")
}

func TestNewAuthService(t *testing.T) {
	t.Run("Positive: valid userRepo and AuthConfig", func(t *testing.T) {
		repo := &mockUserRepo{}
		cfg := config.AuthConfig{JWTSecret: "secret", TokenExpiryHours: 24}
		svc := NewAuthService(repo, cfg)
		if svc == nil || svc.userRepo == nil {
			t.Errorf("expected non-nil AuthService")
		}
	})

	t.Run("Negative 1: nil userRepo dependency", func(t *testing.T) {
		cfg := config.AuthConfig{JWTSecret: "secret"}
		svc := NewAuthService(nil, cfg)
		if svc == nil {
			t.Errorf("expected non-nil struct instance")
		}
		if svc.userRepo != nil {
			t.Errorf("expected nil userRepo")
		}
	})

	t.Run("Negative 2: zero AuthConfig struct", func(t *testing.T) {
		repo := &mockUserRepo{}
		svc := NewAuthService(repo, config.AuthConfig{})
		if svc == nil {
			t.Errorf("expected non-nil AuthService")
		}
		if svc.cfg.JWTSecret != "" {
			t.Errorf("expected empty JWTSecret")
		}
	})
}

func TestAuthService_Register(t *testing.T) {
	cfg := config.AuthConfig{JWTSecret: "secret", TokenExpiryHours: 24}

	t.Run("Positive: successful user registration", func(t *testing.T) {
		repo := &mockUserRepo{
			createUserFunc: func(ctx context.Context, user *domain.User) error {
				user.ID = uuid.New()
				return nil
			},
		}
		svc := NewAuthService(repo, cfg)
		resp, err := svc.Register(context.Background(), domain.RegisterRequest{
			Username: "alice",
			Email:    "alice@example.com",
			Password: "password123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Username != "alice" || resp.Email != "alice@example.com" {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("Negative 1: missing required fields", func(t *testing.T) {
		repo := &mockUserRepo{}
		svc := NewAuthService(repo, cfg)
		_, err := svc.Register(context.Background(), domain.RegisterRequest{
			Username: "",
			Email:    "alice@example.com",
			Password: "password123",
		})
		if err == nil {
			t.Errorf("expected error for missing username")
		}
	})

	t.Run("Negative 2: duplicate user error from repo", func(t *testing.T) {
		repo := &mockUserRepo{
			createUserFunc: func(ctx context.Context, user *domain.User) error {
				return domain.ErrUserAlreadyExists
			},
		}
		svc := NewAuthService(repo, cfg)
		_, err := svc.Register(context.Background(), domain.RegisterRequest{
			Username: "alice",
			Email:    "alice@example.com",
			Password: "password123",
		})
		if !errors.Is(err, domain.ErrUserAlreadyExists) {
			t.Errorf("expected ErrUserAlreadyExists, got: %v", err)
		}
	})
}

func TestAuthService_Login(t *testing.T) {
	cfg := config.AuthConfig{JWTSecret: "secret", TokenExpiryHours: 24}
	password := "password123"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	validUser := &domain.User{
		ID:           uuid.New(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	t.Run("Positive: successful login with correct credentials", func(t *testing.T) {
		repo := &mockUserRepo{
			getUserByUsernameFunc: func(ctx context.Context, username string) (*domain.User, error) {
				return validUser, nil
			},
		}
		svc := NewAuthService(repo, cfg)
		resp, err := svc.Login(context.Background(), domain.LoginRequest{
			Username: "alice",
			Password: password,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Token == "" {
			t.Errorf("expected non-empty JWT token")
		}
	})

	t.Run("Negative 1: empty credentials provided", func(t *testing.T) {
		repo := &mockUserRepo{}
		svc := NewAuthService(repo, cfg)
		_, err := svc.Login(context.Background(), domain.LoginRequest{
			Username: "",
			Password: password,
		})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got: %v", err)
		}
	})

	t.Run("Negative 2: wrong password provided", func(t *testing.T) {
		repo := &mockUserRepo{
			getUserByUsernameFunc: func(ctx context.Context, username string) (*domain.User, error) {
				return validUser, nil
			},
		}
		svc := NewAuthService(repo, cfg)
		_, err := svc.Login(context.Background(), domain.LoginRequest{
			Username: "alice",
			Password: "wrongpassword",
		})
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got: %v", err)
		}
	})
}
