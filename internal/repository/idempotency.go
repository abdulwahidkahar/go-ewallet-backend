package repository

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"

	"github.com/lib/pq"
)

var ErrIdempotencyKeyConflict = errors.New("idempotency key already used with different payload")

type queryRowExecer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type IdempotencyRepository struct {
	db *sql.DB
}

func NewIdempotencyRepository(db *sql.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) Reserve(ctx context.Context, runner queryRowExecer, userID int, scope, key, requestHash string) (model.IdempotencyReserveResult, error) {
	runner = r.resolveRunner(runner)

	record, err := r.insert(ctx, runner, userID, scope, key, requestHash)
	if err == nil {
		return model.IdempotencyReserveResult{
			Record: record,
			IsNew:  true,
		}, nil
	}

	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
		return model.IdempotencyReserveResult{}, err
	}

	record, err = r.GetByUserScopeKey(ctx, runner, userID, scope, key)
	if err != nil {
		return model.IdempotencyReserveResult{}, err
	}

	if record.RequestHash != requestHash {
		return model.IdempotencyReserveResult{}, ErrIdempotencyKeyConflict
	}

	if err := r.Touch(ctx, runner, record.ID); err != nil {
		return model.IdempotencyReserveResult{}, err
	}

	record, err = r.GetByUserScopeKey(ctx, runner, userID, scope, key)
	if err != nil {
		return model.IdempotencyReserveResult{}, err
	}

	return model.IdempotencyReserveResult{
		Record: record,
		IsNew:  false,
	}, nil
}

func (r *IdempotencyRepository) insert(ctx context.Context, runner queryRowExecer, userID int, scope, key, requestHash string) (model.IdempotencyKey, error) {
	var record model.IdempotencyKey

	err := runner.QueryRowContext(
		ctx,
		`INSERT INTO idempotency_keys (user_id, scope, idempotency_key, request_hash, status, locked_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at`,
		userID,
		scope,
		key,
		requestHash,
		model.IdempotencyStatusProcessing,
	).Scan(
		&record.ID,
		&record.UserID,
		&record.Scope,
		&record.IdempotencyKey,
		&record.RequestHash,
		&record.Status,
		&record.ResourceType,
		&record.ResourceID,
		&record.LockedAt,
		&record.LastSeenAt,
		&record.ExpiresAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return model.IdempotencyKey{}, err
	}

	return record, nil
}

func (r *IdempotencyRepository) GetByUserScopeKey(ctx context.Context, runner queryRowExecer, userID int, scope, key string) (model.IdempotencyKey, error) {
	runner = r.resolveRunner(runner)

	var record model.IdempotencyKey

	err := runner.QueryRowContext(
		ctx,
		`SELECT id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at
		FROM idempotency_keys
		WHERE user_id = $1 AND scope = $2 AND idempotency_key = $3`,
		userID,
		scope,
		key,
	).Scan(
		&record.ID,
		&record.UserID,
		&record.Scope,
		&record.IdempotencyKey,
		&record.RequestHash,
		&record.Status,
		&record.ResourceType,
		&record.ResourceID,
		&record.LockedAt,
		&record.LastSeenAt,
		&record.ExpiresAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return model.IdempotencyKey{}, err
	}

	return record, nil
}

func (r *IdempotencyRepository) Touch(ctx context.Context, runner queryRowExecer, id int) error {
	runner = r.resolveRunner(runner)

	_, err := runner.ExecContext(
		ctx,
		`UPDATE idempotency_keys
		SET last_seen_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		id,
	)

	return err
}

func (r *IdempotencyRepository) MarkCompleted(ctx context.Context, runner queryRowExecer, id int, resourceType string, resourceID int) error {
	runner = r.resolveRunner(runner)

	_, err := runner.ExecContext(
		ctx,
		`UPDATE idempotency_keys
		SET status = $2,
			resource_type = $3,
			resource_id = $4,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		id,
		model.IdempotencyStatusCompleted,
		resourceType,
		resourceID,
	)

	return err
}

func (r *IdempotencyRepository) MarkFailed(ctx context.Context, runner queryRowExecer, id int) error {
	runner = r.resolveRunner(runner)

	_, err := runner.ExecContext(
		ctx,
		`UPDATE idempotency_keys
		SET status = $2,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
		id,
		model.IdempotencyStatusFailed,
	)

	return err
}

func (r *IdempotencyRepository) resolveRunner(runner queryRowExecer) queryRowExecer {
	if runner != nil {
		return runner
	}

	return r.db
}
