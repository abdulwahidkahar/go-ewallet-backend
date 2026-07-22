package repository

import (
	"context"
	"database/sql"
	"go-ewallet-backend/internal/model"
)

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// CreateTx inserts a new outbox event within the given transaction.
// This ensures the event is atomically committed with the business logic.
func (r *OutboxRepository) CreateTx(ctx context.Context, tx *sql.Tx, event model.OutboxEvent) (int64, error) {
	var id int64

	err := tx.QueryRowContext(
		ctx,
		`INSERT INTO outbox_events (event_type, payload, status)
		VALUES ($1, $2, $3)
		RETURNING id`,
		event.EventType,
		event.Payload,
		model.OutboxStatusPending,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// GetPendingForPublish retrieves up to `limit` pending events, locking them
// with FOR UPDATE SKIP LOCKED to allow safe concurrent worker instances.
// Uses exponential backoff: events that have been retried are skipped until
// their backoff window expires (1s, 2s, 4s, ... capped at 300s).
func (r *OutboxRepository) GetPendingForPublish(ctx context.Context, tx *sql.Tx, limit int) ([]model.OutboxEvent, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, event_type, payload, status, retry_count, last_error, created_at, updated_at, published_at
		FROM outbox_events
		WHERE status = $1
		  AND (retry_count = 0 OR updated_at + make_interval(secs => LEAST(POWER(2, retry_count), 300)) <= CURRENT_TIMESTAMP)
		ORDER BY created_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED`,
		model.OutboxStatusPending,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.OutboxEvent
	for rows.Next() {
		var e model.OutboxEvent
		if err := rows.Scan(
			&e.ID,
			&e.EventType,
			&e.Payload,
			&e.Status,
			&e.RetryCount,
			&e.LastError,
			&e.CreatedAt,
			&e.UpdatedAt,
			&e.PublishedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// MarkPublished updates the event status to PUBLISHED and sets published_at.
func (r *OutboxRepository) MarkPublished(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE outbox_events
		SET status = $2,
			published_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		id,
		model.OutboxStatusPublished,
	)
	return err
}

// RecordRetry increments the retry count and records the error message.
// The event stays PENDING and will be retried with exponential backoff.
// Events are never marked as FAILED automatically — they retry forever.
func (r *OutboxRepository) RecordRetry(ctx context.Context, tx *sql.Tx, id int64, errMsg string) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE outbox_events
		SET retry_count = retry_count + 1,
			last_error = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		id,
		errMsg,
	)
	return err
}
