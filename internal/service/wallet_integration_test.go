package service

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"

	_ "github.com/lib/pq"
)

func TestWalletServiceTransferWithIdempotency_ConcurrentTransfersOnlyOneSucceeds(t *testing.T) {
	baseDSN := os.Getenv("EWALLET_TEST_DSN")
	if baseDSN == "" {
		t.Skip("set EWALLET_TEST_DSN to run concurrent transfer integration test")
	}

	schemaName := fmt.Sprintf("ewallet_test_%d", time.Now().UnixNano())

	adminDB, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("failed to open admin db: %v", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(`CREATE SCHEMA ` + schemaName); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	defer adminDB.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`)

	testDSN, err := withSearchPath(baseDSN, schemaName)
	if err != nil {
		t.Fatalf("failed to build dsn with search_path: %v", err)
	}

	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	if err := createConcurrentTransferTestSchema(db); err != nil {
		t.Fatalf("failed to create test schema objects: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO users (id, email, password) VALUES
		(1, 'sender@example.com', 'secret'),
		(2, 'recipient@example.com', 'secret')`); err != nil {
		t.Fatalf("failed to seed users: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO wallets (id, user_id, balance, currency) VALUES
		(1, 1, 50000, 'IDR'),
		(2, 2, 0, 'IDR')`); err != nil {
		t.Fatalf("failed to seed wallets: %v", err)
	}

	walletRepo := repository.NewWalletRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	service := NewWalletService(db, walletRepo, nil, ledgerRepo, NewIdempotencyService(idempotencyRepo))

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup

	runTransfer := func(key string) {
		defer wg.Done()
		<-start
		_, err := service.TransferWithIdempotency(context.Background(), 1, key, model.TransferRequest{
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

	var senderBalance int64
	if err := db.QueryRow(`SELECT balance FROM wallets WHERE id = 1`).Scan(&senderBalance); err != nil {
		t.Fatalf("failed to query sender balance: %v", err)
	}
	if senderBalance != 10000 {
		t.Fatalf("expected sender balance 10000, got %d", senderBalance)
	}

	var recipientBalance int64
	if err := db.QueryRow(`SELECT balance FROM wallets WHERE id = 2`).Scan(&recipientBalance); err != nil {
		t.Fatalf("failed to query recipient balance: %v", err)
	}
	if recipientBalance != 40000 {
		t.Fatalf("expected recipient balance 40000, got %d", recipientBalance)
	}

	var transferCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&transferCount); err != nil {
		t.Fatalf("failed to query transfer count: %v", err)
	}
	if transferCount != 1 {
		t.Fatalf("expected 1 transfer row, got %d", transferCount)
	}

	var ledgerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_entries`).Scan(&ledgerCount); err != nil {
		t.Fatalf("failed to query ledger count: %v", err)
	}
	if ledgerCount != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", ledgerCount)
	}
}

func withSearchPath(baseDSN, schema string) (string, error) {
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String(), nil
}

func createConcurrentTransferTestSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE wallets (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL UNIQUE,
			balance BIGINT NOT NULL DEFAULT 0,
			currency VARCHAR(3) NOT NULL DEFAULT 'IDR',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE idempotency_keys (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL,
			scope VARCHAR(50) NOT NULL,
			idempotency_key VARCHAR(255) NOT NULL,
			request_hash VARCHAR(64) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'processing',
			resource_type VARCHAR(50),
			resource_id INT,
			locked_at TIMESTAMP WITH TIME ZONE,
			last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (user_id, scope, idempotency_key)
		)`,
		`CREATE TABLE transfers (
			id SERIAL PRIMARY KEY,
			from_wallet_id INT NOT NULL,
			to_wallet_id INT NOT NULL,
			amount BIGINT NOT NULL,
			reference_id VARCHAR(100) NOT NULL UNIQUE,
			status VARCHAR(20) NOT NULL DEFAULT 'success',
			description TEXT,
			idempotency_key_id INT UNIQUE,
			sender_balance_before BIGINT,
			sender_balance_after BIGINT,
			recipient_balance_before BIGINT,
			recipient_balance_after BIGINT,
			created_by_user_id INT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (from_wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
			FOREIGN KEY (to_wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
			FOREIGN KEY (idempotency_key_id) REFERENCES idempotency_keys(id) ON DELETE SET NULL,
			FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE ledger_entries (
			id SERIAL PRIMARY KEY,
			wallet_id INT NOT NULL,
			reference_id VARCHAR(100) NOT NULL,
			transaction_type VARCHAR(50) NOT NULL,
			direction VARCHAR(10) NOT NULL,
			amount BIGINT NOT NULL,
			balance_before BIGINT NOT NULL,
			balance_after BIGINT NOT NULL,
			description TEXT,
			transfer_id INT,
			top_up_order_id INT,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE,
			FOREIGN KEY (transfer_id) REFERENCES transfers(id) ON DELETE SET NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func Test_withSearchPath_AppendsSearchPathParameter(t *testing.T) {
	dsn, err := withSearchPath("postgres://user:pass@localhost:5432/dbname?sslmode=disable", "schema_a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dsn, "search_path=schema_a") {
		t.Fatalf("expected search_path parameter in dsn, got %s", dsn)
	}
}
