package repository

import (
	"context"
	"go-ewallet-backend/internal/model"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOutboxRepository_CreateTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewOutboxRepository(db)

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO outbox_events (event_type, payload, status)
		VALUES ($1, $2, $3)
		RETURNING id`,
	)).
		WithArgs("transfer.success", []byte(`{"amount":10000}`), model.OutboxStatusPending).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	id, err := repo.CreateTx(context.Background(), tx, model.OutboxEvent{
		EventType: "transfer.success",
		Payload:   []byte(`{"amount":10000}`),
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestOutboxRepository_GetPendingForPublish(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewOutboxRepository(db)
	now := time.Now()

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, event_type, payload, status, retry_count, last_error, created_at, updated_at, published_at
		FROM outbox_events
		WHERE status = $1
		  AND (retry_count = 0 OR updated_at + make_interval(secs => LEAST(POWER(2, retry_count), 300)) <= CURRENT_TIMESTAMP)
		ORDER BY created_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`,
	)).
		WithArgs(model.OutboxStatusPending, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "event_type", "payload", "status", "retry_count", "last_error", "created_at", "updated_at", "published_at",
		}).
			AddRow(int64(1), "transfer.success", []byte(`{"amount":10000}`), model.OutboxStatusPending, 0, nil, now, now, nil).
			AddRow(int64(2), "topup.confirmed", []byte(`{"amount":50000}`), model.OutboxStatusPending, 1, "redis timeout", now, now, nil))

	mock.ExpectRollback()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	events, err := repo.GetPendingForPublish(context.Background(), tx, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "transfer.success" {
		t.Fatalf("expected transfer.success, got %s", events[0].EventType)
	}
	if events[1].RetryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", events[1].RetryCount)
	}

	tx.Rollback()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewOutboxRepository(db)

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE outbox_events
		SET status = $2,
			published_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
	)).
		WithArgs(int64(1), model.OutboxStatusPublished).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	err = repo.MarkPublished(context.Background(), tx, 1)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestOutboxRepository_RecordRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewOutboxRepository(db)

	mock.ExpectBegin()

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE outbox_events
		SET retry_count = retry_count + 1,
			last_error = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
	)).
		WithArgs(int64(1), "redis connection refused").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	err = repo.RecordRetry(context.Background(), tx, 1, "redis connection refused")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestOutboxRepository_GetPendingForPublish_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewOutboxRepository(db)

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, event_type, payload, status, retry_count, last_error, created_at, updated_at, published_at
		FROM outbox_events
		WHERE status = $1
		  AND (retry_count = 0 OR updated_at + make_interval(secs => LEAST(POWER(2, retry_count), 300)) <= CURRENT_TIMESTAMP)
		ORDER BY created_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`,
	)).
		WithArgs(model.OutboxStatusPending, 100).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "event_type", "payload", "status", "retry_count", "last_error", "created_at", "updated_at", "published_at",
		}))

	mock.ExpectRollback()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	events, err := repo.GetPendingForPublish(context.Background(), tx, 100)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}

	tx.Rollback()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
