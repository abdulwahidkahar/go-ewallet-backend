package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestIdempotencyServiceReserve_ReturnsConflictForDifferentPayload(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := repository.NewIdempotencyRepository(db)
	service := NewIdempotencyService(repo)

	now := time.Now()

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO idempotency_keys (user_id, scope, idempotency_key, request_hash, status, locked_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTransfer, "dup-key", sqlmock.AnyArg(), model.IdempotencyStatusProcessing).
		WillReturnError(&pq.Error{Code: "23505"})

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at
		FROM idempotency_keys
		WHERE user_id = $1 AND scope = $2 AND idempotency_key = $3`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTransfer, "dup-key").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "scope", "idempotency_key", "request_hash", "status", "resource_type", "resource_id", "locked_at", "last_seen_at", "expires_at", "created_at", "updated_at",
		}).AddRow(
			10,
			1,
			model.IdempotencyScopeWalletTransfer,
			"dup-key",
			"different-hash",
			model.IdempotencyStatusCompleted,
			sql.NullString{},
			sql.NullInt64{},
			sql.NullTime{Time: now, Valid: true},
			now,
			sql.NullTime{},
			now,
			now,
		))

	_, err = service.Reserve(context.Background(), tx, 1, model.IdempotencyScopeWalletTransfer, "dup-key", model.TransferRequest{
		ToWalletID:  2,
		Amount:      15000,
		Description: "duplicate request",
	})
	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		t.Fatalf("expected ErrIdempotencyKeyConflict, got %v", err)
	}

	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatalf("failed to rollback tx: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
