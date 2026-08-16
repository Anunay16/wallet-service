package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/anunay/wallet-service/internal/domain"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type mockAuthService struct {
	registerFunc func(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error)
	loginFunc    func(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error)
}

func (m *mockAuthService) Register(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockAuthService) Login(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func TestNewAuthHandler(t *testing.T) {
	t.Run("Positive: valid service dependency", func(t *testing.T) {
		mockSvc := &mockAuthService{}
		h := NewAuthHandler(mockSvc)
		if h == nil || h.authService == nil {
			t.Errorf("expected non-nil AuthHandler")
		}
	})

	t.Run("Negative 1: nil service dependency", func(t *testing.T) {
		h := NewAuthHandler(nil)
		if h == nil {
			t.Fatalf("expected struct instance even with nil dependency")
		}
		if h.authService != nil {
			t.Errorf("expected nil authService")
		}
	})

	t.Run("Negative 2: uninitialized mock service dependency", func(t *testing.T) {
		var mockSvc *mockAuthService
		h := NewAuthHandler(mockSvc)
		if h == nil {
			t.Errorf("expected non-nil AuthHandler")
		}
	})
}

func TestAuthHandler_Register(t *testing.T) {
	t.Run("Positive: successful user registration", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{
			registerFunc: func(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error) {
				return &domain.UserResponse{
					ID:       uuid.New(),
					Username: req.Username,
					Email:    req.Email,
				}, nil
			},
		}
		h := NewAuthHandler(mockSvc)
		app.Post("/register", h.Register)

		body, _ := json.Marshal(domain.RegisterRequest{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "password123",
		})
		req := httptest.NewRequest("POST", "/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusCreated {
			t.Errorf("expected status 201, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 1: invalid request body", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{}
		h := NewAuthHandler(mockSvc)
		app.Post("/register", h.Register)

		req := httptest.NewRequest("POST", "/register", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: user already exists conflict", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{
			registerFunc: func(ctx context.Context, req domain.RegisterRequest) (*domain.UserResponse, error) {
				return nil, domain.ErrUserAlreadyExists
			},
		}
		h := NewAuthHandler(mockSvc)
		app.Post("/register", h.Register)

		body, _ := json.Marshal(domain.RegisterRequest{
			Username: "existinguser",
			Email:    "existing@example.com",
			Password: "password123",
		})
		req := httptest.NewRequest("POST", "/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusConflict {
			t.Errorf("expected status 409, got: %d", resp.StatusCode)
		}
	})
}

func TestAuthHandler_Login(t *testing.T) {
	t.Run("Positive: successful login", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{
			loginFunc: func(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
				return &domain.LoginResponse{Token: "test.jwt.token"}, nil
			},
		}
		h := NewAuthHandler(mockSvc)
		app.Post("/login", h.Login)

		body, _ := json.Marshal(domain.LoginRequest{
			Username: "testuser",
			Password: "password123",
		})
		req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("expected status 200, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 1: invalid request body", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{}
		h := NewAuthHandler(mockSvc)
		app.Post("/login", h.Login)

		req := httptest.NewRequest("POST", "/login", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("expected status 400, got: %d", resp.StatusCode)
		}
	})

	t.Run("Negative 2: invalid credentials error", func(t *testing.T) {
		app := fiber.New()
		mockSvc := &mockAuthService{
			loginFunc: func(ctx context.Context, req domain.LoginRequest) (*domain.LoginResponse, error) {
				return nil, domain.ErrInvalidCredentials
			},
		}
		h := NewAuthHandler(mockSvc)
		app.Post("/login", h.Login)

		body, _ := json.Marshal(domain.LoginRequest{
			Username: "testuser",
			Password: "wrongpassword",
		})
		req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected status 401, got: %d", resp.StatusCode)
		}
	})
}
