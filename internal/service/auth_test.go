package service

import (
	"context"
	"testing"

	"go-ewallet-backend/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAuthServiceLogin_ReturnsInvalidCredentialsForUnknownEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	service := NewAuthService(db, repository.NewUserRepository(db), nil)

	mock.ExpectQuery("SELECT id, password FROM users WHERE email = \\$1").
		WithArgs("missing@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id", "password"}))

	_, err = service.Login(context.Background(), "missing@example.com", "secret123")
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

	service := NewAuthService(db, repository.NewUserRepository(db), nil)

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
