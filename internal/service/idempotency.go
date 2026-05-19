package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
)

var ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
var ErrIdempotencyKeyConflict = errors.New("idempotency key already used with different payload")
var ErrIdempotencyRequestInProgress = errors.New("request with this idempotency key is still processing")
var ErrIdempotencyRequestFailed = errors.New("previous request with this idempotency key failed")

type IdempotencyService struct {
	repo *repository.IdempotencyRepository
}

func NewIdempotencyService(repo *repository.IdempotencyRepository) *IdempotencyService {
	return &IdempotencyService{repo: repo}
}

func (s *IdempotencyService) Reserve(ctx context.Context, tx *sql.Tx, userID int, scope, key string, payload any) (model.IdempotencyReserveResult, error) {
	if key == "" {
		return model.IdempotencyReserveResult{}, ErrIdempotencyKeyRequired
	}

	requestHash, err := hashPayload(payload)
	if err != nil {
		return model.IdempotencyReserveResult{}, err
	}

	result, err := s.repo.Reserve(ctx, tx, userID, scope, key, requestHash)
	if errors.Is(err, repository.ErrIdempotencyKeyConflict) {
		return model.IdempotencyReserveResult{}, ErrIdempotencyKeyConflict
	}

	return result, err
}

func (s *IdempotencyService) Complete(ctx context.Context, tx *sql.Tx, id int, resourceType string, resourceID int) error {
	return s.repo.MarkCompleted(ctx, tx, id, resourceType, resourceID)
}

func (s *IdempotencyService) Fail(ctx context.Context, tx *sql.Tx, id int) error {
	return s.repo.MarkFailed(ctx, tx, id)
}

func hashPayload(payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
