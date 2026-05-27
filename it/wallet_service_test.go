//go:build integration

package it

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
	"go-ewallet-backend/internal/service"
)

func TestWalletServiceTransferWithIdempotency_ConcurrentTransfersOnlyOneSucceeds(t *testing.T) {
	store := newIntegrationDB(t)
	seedWalletTransferFixture(t, store.db)

	svc := newWalletService(store.db)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup

	runTransfer := func(key string) {
		defer wg.Done()
		<-start
		_, err := svc.TransferWithIdempotency(context.Background(), 1, key, model.TransferRequest{
			ToWalletID:  2,
			Amount:      40000,
			Description: "concurrent test",
		})
		results <- err
	}

	wg.Add(2)
	go runTransfer("concurrent-transfer-1")
	go runTransfer("concurrent-transfer-2")

	close(start)
	wg.Wait()
	close(results)

	var successCount int
	var insufficientCount int

	for err := range results {
		switch {
		case err == nil:
			successCount++
		case err.Error() == "insufficient balance":
			insufficientCount++
		default:
			t.Fatalf("unexpected transfer error: %v", err)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly one successful transfer, got %d", successCount)
	}
	if insufficientCount != 1 {
		t.Fatalf("expected exactly one insufficient balance error, got %d", insufficientCount)
	}

	assertWalletBalance(t, store.db, 1, 10000)
	assertWalletBalance(t, store.db, 2, 40000)
	assertCount(t, store.db, `SELECT COUNT(*) FROM transfers`, 1)
	assertCount(t, store.db, `SELECT COUNT(*) FROM ledger_entries`, 2)
}

func newWalletService(db *sql.DB) *service.WalletService {
	walletRepo := repository.NewWalletRepository(db)
	topUpRepo := repository.NewTopUpRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)

	return service.NewWalletService(
		db,
		walletRepo,
		topUpRepo,
		ledgerRepo,
		service.NewIdempotencyService(idempotencyRepo),
	)
}

func seedWalletTransferFixture(t *testing.T, db execer) {
	t.Helper()

	seedUser(t, db, 1, "sender@example.com")
	seedUser(t, db, 2, "recipient@example.com")

	if _, err := db.Exec(`INSERT INTO wallets (id, user_id, balance, currency) VALUES
		(1, 1, 50000, 'IDR'),
		(2, 2, 0, 'IDR')`); err != nil {
		t.Fatalf("failed to seed wallets: %v", err)
	}
}

func assertWalletBalance(t *testing.T, db queryRower, walletID int, expected int64) {
	t.Helper()

	var balance int64
	if err := db.QueryRow(`SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance); err != nil {
		t.Fatalf("failed to query wallet %d balance: %v", walletID, err)
	}
	if balance != expected {
		t.Fatalf("expected wallet %d balance %d, got %d", walletID, expected, balance)
	}
}

func assertCount(t *testing.T, db queryRower, query string, expected int) {
	t.Helper()

	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("failed to count rows for %q: %v", query, err)
	}
	if count != expected {
		t.Fatalf("expected count %d for %q, got %d", expected, query, count)
	}
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}
