package service

import (
	"context"
	"testing"
	"time"

	"go-ewallet-backend/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAuthServiceLogin_ReturnsInvalidCredentialsForUnknownEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	service := NewAuthService(db, repository.NewUserRepository(db), nil, nil)

	mock.ExpectQuery("SELECT id, password FROM users WHERE email = \\$1").
		WithArgs("missing@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password"}))

	_, err = service.Login(context.Background(), "missing@example.com", "secret123", "", "", "")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestAuthServiceProfile_ReturnsUserNotFoundWhenEmailEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	service := NewAuthService(db, repository.NewUserRepository(db), nil, nil)

	mock.ExpectQuery("SELECT email, password FROM users WHERE id = \\$1").
		WithArgs(999).
		WillReturnRows(sqlmock.NewRows([]string{"email", "password"}))

	_, err = service.Profile(context.Background(), 999)
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestHashToken_Consistent(t *testing.T) {
	token := "sample_raw_refresh_token_123"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 != hash2 {
		t.Fatalf("expected consistent hash, got %s and %s", hash1, hash2)
	}
	if hash1 == token {
		t.Fatalf("hash should not equal raw token")
	}
}

func TestAuthServiceRefresh_Rotation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	t.Setenv("JWT_SECRET", "testsecretkey")

	refreshRepo := repository.NewRefreshTokenRepository(db)
	userRepo := repository.NewUserRepository(db)
	authService := NewAuthService(db, userRepo, nil, refreshRepo)

	rawToken := "raw_test_refresh_token"
	hashedToken := HashToken(rawToken)

	// Mock GetByHash
	mock.ExpectQuery("SELECT id, user_id, token_hash, device_id, user_agent, ip_address, is_revoked, expires_at, created_at, last_used_at FROM refresh_tokens WHERE token_hash = \\$1").
		WithArgs(hashedToken).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token_hash", "device_id", "user_agent", "ip_address", "is_revoked", "expires_at", "created_at", "last_used_at"}).
			AddRow(1, 10, hashedToken, "dev-1", "UA-Test", "127.0.0.1", false, time.Now().Add(1*time.Hour), time.Now(), time.Now()))

	// Mock RevokeByHash (Rotation)
	mock.ExpectExec("UPDATE refresh_tokens SET is_revoked = TRUE WHERE token_hash = \\$1").
		WithArgs(hashedToken).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Mock GetByID (User info for JWT creation)
	mock.ExpectQuery("SELECT email, password FROM users WHERE id = \\$1").
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"email", "password"}).AddRow("testuser@example.com", "hash"))

	// Mock Create new refresh token
	mock.ExpectQuery("INSERT INTO refresh_tokens").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))

	resp, err := authService.Refresh(context.Background(), rawToken, "dev-1", "UA-Test", "127.0.0.1")
	if err != nil {
		t.Fatalf("expected successful refresh, got error: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("expected non-empty tokens in response, got access: %s, refresh: %s", resp.AccessToken, resp.RefreshToken)
	}

	if resp.RefreshToken == rawToken {
		t.Fatalf("expected new refresh token after rotation, got same token")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestAuthServiceRefresh_RevokedTokenReused_RevokesAllSessions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	refreshRepo := repository.NewRefreshTokenRepository(db)
	authService := NewAuthService(db, nil, nil, refreshRepo)

	rawToken := "reused_revoked_token"
	hashedToken := HashToken(rawToken)

	// Mock GetByHash returning is_revoked = true
	mock.ExpectQuery("SELECT id, user_id, token_hash, device_id, user_agent, ip_address, is_revoked, expires_at, created_at, last_used_at FROM refresh_tokens WHERE token_hash = \\$1").
		WithArgs(hashedToken).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token_hash", "device_id", "user_agent", "ip_address", "is_revoked", "expires_at", "created_at", "last_used_at"}).
			AddRow(1, 10, hashedToken, "dev-1", "UA-Test", "127.0.0.1", true, time.Now().Add(1*time.Hour), time.Now(), time.Now()))

	// Expect RevokeAllByUserID for security
	mock.ExpectExec("UPDATE refresh_tokens SET is_revoked = TRUE WHERE user_id = \\$1 AND is_revoked = FALSE").
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	_, err = authService.Refresh(context.Background(), rawToken, "", "", "")
	if err != ErrRefreshTokenRevoked {
		t.Fatalf("expected ErrRefreshTokenRevoked, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
