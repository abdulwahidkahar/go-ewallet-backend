//go:build integration

package it

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
)

func TestIdempotencyRepositoryReserve_ReturnsExistingRecordForDuplicateRequest(t *testing.T) {
	store := newIntegrationDB(t)
	seedUser(t, store.db, 1, "arya@example.com")

	repo := repository.NewIdempotencyRepository(store.db)

	first, err := repo.Reserve(context.Background(), nil, 1, model.IdempotencyScopeWalletTransfer, "dup-key", "hash-a")
	if err != nil {
		t.Fatalf("failed to reserve first request: %v", err)
	}
	if !first.IsNew {
		t.Fatal("expected first reservation to be new")
	}

	time.Sleep(10 * time.Millisecond)

	second, err := repo.Reserve(context.Background(), nil, 1, model.IdempotencyScopeWalletTransfer, "dup-key", "hash-a")
	if err != nil {
		t.Fatalf("failed to reserve duplicate request: %v", err)
	}
	if second.IsNew {
		t.Fatal("expected duplicate reservation to reuse existing record")
	}
	if second.Record.ID != first.Record.ID {
		t.Fatalf("expected duplicate reservation to return record %d, got %d", first.Record.ID, second.Record.ID)
	}
	if second.Record.LastSeenAt.Before(first.Record.LastSeenAt) {
		t.Fatalf("expected last_seen_at to move forward, first=%s second=%s", first.Record.LastSeenAt, second.Record.LastSeenAt)
	}
}

func TestIdempotencyRepositoryReserve_ReturnsConflictForDifferentPayload(t *testing.T) {
	store := newIntegrationDB(t)
	seedUser(t, store.db, 1, "arya@example.com")

	repo := repository.NewIdempotencyRepository(store.db)

	if _, err := repo.Reserve(context.Background(), nil, 1, model.IdempotencyScopeWalletTransfer, "dup-key", "hash-a"); err != nil {
		t.Fatalf("failed to reserve first request: %v", err)
	}

	_, err := repo.Reserve(context.Background(), nil, 1, model.IdempotencyScopeWalletTransfer, "dup-key", "hash-b")
	if !errors.Is(err, repository.ErrIdempotencyKeyConflict) {
		t.Fatalf("expected ErrIdempotencyKeyConflict, got %v", err)
	}
}

func seedUser(t *testing.T, db execer, id int, email string) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO users (id, email, password) VALUES ($1, $2, $3)`,
		id,
		email,
		"secret",
	); err != nil {
		t.Fatalf("failed to seed user %d: %v", id, err)
	}
}
