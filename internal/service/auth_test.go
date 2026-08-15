package service

import (
	"context"
	"testing"
	"time"

	"github.com/anunay/wallet-service/config"
	"github.com/anunay/wallet-service/internal/domain"
	"github.com/google/uuid"
)

type mockUserRepo struct {
	users map[string]*domain.User
}

func (m *mockUserRepo) CreateUser(ctx context.Context, user *domain.User) error {
	if _, ok := m.users[user.Username]; ok {
		return domain.ErrUserAlreadyExists
	}
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.Username] = user
	return nil
}

func (m *mockUserRepo) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func TestAuthService_RegisterAndLogin(t *testing.T) {
	repo := &mockUserRepo{users: make(map[string]*domain.User)}
	cfg := config.AuthConfig{JWTSecret: "test-secret", TokenExpiryHours: 24}
	authSvc := NewAuthService(repo, cfg)

	ctx := context.Background()

	// 1. Successful Register
	regReq := domain.RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "password123",
	}
	userResp, err := authSvc.Register(ctx, regReq)
	if err != nil {
		t.Fatalf("expected no error on register, got: %v", err)
	}
	if userResp.Username != "alice" {
		t.Errorf("expected username alice, got: %s", userResp.Username)
	}

	// 2. Duplicate Register
	_, err = authSvc.Register(ctx, regReq)
	if err != domain.ErrUserAlreadyExists {
		t.Errorf("expected ErrUserAlreadyExists, got: %v", err)
	}

	// 3. Successful Login
	loginResp, err := authSvc.Login(ctx, domain.LoginRequest{
		Username: "alice",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("expected no error on login, got: %v", err)
	}
	if loginResp.Token == "" {
		t.Errorf("expected non-empty token")
	}

	// 4. Invalid Password Login
	_, err = authSvc.Login(ctx, domain.LoginRequest{
		Username: "alice",
		Password: "wrongpassword",
	})
	if err != domain.ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}
