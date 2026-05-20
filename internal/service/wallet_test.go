package service

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestWalletServiceTransferWithIdempotency_ReturnsErrorForSameWallet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	walletRepo := repository.NewWalletRepository(db)
	service := NewWalletService(db, walletRepo, nil, nil, nil)

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "balance", "currency", "created_at", "updated_at",
	}).AddRow(7, 1, int64(50000), "IDR", "2026-05-19 09:00:00+00", "2026-05-19 09:00:00+00")

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, user_id, balance, currency, created_at, updated_at FROM wallets WHERE user_id = $1",
	)).
		WithArgs(1).
		WillReturnRows(rows)

	_, err = service.TransferWithIdempotency(context.Background(), 1, "transfer-key-1", model.TransferRequest{
		ToWalletID: 7,
		Amount:     10000,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "cannot transfer to the same wallet" {
		t.Fatalf("expected same wallet error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestWalletServiceTransferWithIdempotency_ReturnsExistingTransferForSuccessfulRetry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	walletRepo := repository.NewWalletRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	service := NewWalletService(db, walletRepo, nil, nil, NewIdempotencyService(idempotencyRepo))

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "balance", "currency", "created_at", "updated_at",
	}).AddRow(1, 1, int64(50000), "IDR", "2026-05-19 09:00:00+00", "2026-05-19 09:00:00+00")

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, user_id, balance, currency, created_at, updated_at FROM wallets WHERE user_id = $1",
	)).
		WithArgs(1).
		WillReturnRows(rows)

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO idempotency_keys (user_id, scope, idempotency_key, request_hash, status, locked_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTransfer, "retry-key-1", sqlmock.AnyArg(), model.IdempotencyStatusProcessing).
		WillReturnError(&pq.Error{Code: "23505"})

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at
		FROM idempotency_keys
		WHERE user_id = $1 AND scope = $2 AND idempotency_key = $3`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTransfer, "retry-key-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "scope", "idempotency_key", "request_hash", "status", "resource_type", "resource_id", "locked_at", "last_seen_at", "expires_at", "created_at", "updated_at",
		}).AddRow(
			9,
			1,
			model.IdempotencyScopeWalletTransfer,
			"retry-key-1",
			mustHashPayload(t, model.TransferRequest{ToWalletID: 2, Amount: 20000, Description: "retry transfer"}),
			model.IdempotencyStatusCompleted,
			sql.NullString{String: "transfer", Valid: true},
			sql.NullInt64{Int64: 77, Valid: true},
			sql.NullTime{Time: now, Valid: true},
			now,
			sql.NullTime{},
			now,
			now,
		))

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE idempotency_keys
		SET last_seen_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
	)).
		WithArgs(9).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at
		FROM idempotency_keys
		WHERE user_id = $1 AND scope = $2 AND idempotency_key = $3`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTransfer, "retry-key-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "scope", "idempotency_key", "request_hash", "status", "resource_type", "resource_id", "locked_at", "last_seen_at", "expires_at", "created_at", "updated_at",
		}).AddRow(
			9,
			1,
			model.IdempotencyScopeWalletTransfer,
			"retry-key-1",
			mustHashPayload(t, model.TransferRequest{ToWalletID: 2, Amount: 20000, Description: "retry transfer"}),
			model.IdempotencyStatusCompleted,
			sql.NullString{String: "transfer", Valid: true},
			sql.NullInt64{Int64: 77, Valid: true},
			sql.NullTime{Time: now, Valid: true},
			now,
			sql.NullTime{},
			now,
			now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, from_wallet_id, to_wallet_id, amount, reference_id, status, description,
			sender_balance_before, sender_balance_after, recipient_balance_before, recipient_balance_after, created_at::text
		FROM transfers
		WHERE id = $1`,
	)).
		WithArgs(77).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "from_wallet_id", "to_wallet_id", "amount", "reference_id", "status", "description",
			"sender_balance_before", "sender_balance_after", "recipient_balance_before", "recipient_balance_after", "created_at",
		}).AddRow(
			77, 1, 2, int64(20000), "TRF-existing", "success", "retry transfer",
			int64(50000), int64(30000), int64(10000), int64(30000), "2026-05-19 09:45:00+00",
		))

	mock.ExpectRollback()

	transfer, err := service.TransferWithIdempotency(context.Background(), 1, "retry-key-1", model.TransferRequest{
		ToWalletID:  2,
		Amount:      20000,
		Description: "retry transfer",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if transfer.ID != 77 {
		t.Fatalf("expected existing transfer id 77, got %d", transfer.ID)
	}
	if transfer.ReferenceID != "TRF-existing" {
		t.Fatalf("expected existing reference id, got %s", transfer.ReferenceID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestWalletServiceCreateTopUp_CreatesPendingOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	walletRepo := repository.NewWalletRepository(db)
	topUpRepo := repository.NewTopUpRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	service := NewWalletService(db, walletRepo, topUpRepo, nil, NewIdempotencyService(idempotencyRepo))

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, user_id, balance, currency, created_at, updated_at FROM wallets WHERE user_id = $1",
	)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "balance", "currency", "created_at", "updated_at",
		}).AddRow(1, 1, int64(0), "IDR", "2026-05-19 09:00:00+00", "2026-05-19 09:00:00+00"))

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO idempotency_keys (user_id, scope, idempotency_key, request_hash, status, locked_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTopUpCreate, "topup-create-1", sqlmock.AnyArg(), model.IdempotencyStatusProcessing).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "scope", "idempotency_key", "request_hash", "status", "resource_type", "resource_id", "locked_at", "last_seen_at", "expires_at", "created_at", "updated_at",
		}).AddRow(
			11, 1, model.IdempotencyScopeWalletTopUpCreate, "topup-create-1", "request-hash",
			model.IdempotencyStatusProcessing, sql.NullString{}, sql.NullInt64{}, sql.NullTime{Time: now, Valid: true}, now, sql.NullTime{}, now, now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO top_up_orders (wallet_id, reference_id, idempotency_key_id, amount, status, payment_channel, description)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id, wallet_id, reference_id, amount, status, payment_channel, external_reference, description, balance_before, balance_after,
			created_at::text, updated_at::text, confirmed_at::text, failed_at::text`,
	)).
		WithArgs(1, sqlmock.AnyArg(), 11, int64(50000), "manual", "Top up pertama").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "wallet_id", "reference_id", "amount", "status", "payment_channel", "external_reference", "description", "balance_before", "balance_after", "created_at", "updated_at", "confirmed_at", "failed_at",
		}).AddRow(
			1, 1, "TUP-generated", int64(50000), "pending", "manual", nil, "Top up pertama", nil, nil,
			"2026-05-19 09:30:00+00", "2026-05-19 09:30:00+00", nil, nil,
		))

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE idempotency_keys
		SET status = $2,
			resource_type = $3,
			resource_id = $4,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
	)).
		WithArgs(11, model.IdempotencyStatusCompleted, "top_up_order", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	order, err := service.CreateTopUp(context.Background(), 1, "topup-create-1", model.CreateTopUpRequest{
		Amount:         50000,
		PaymentChannel: "manual",
		Description:    "Top up pertama",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if order.Status != "pending" {
		t.Fatalf("expected pending order, got %s", order.Status)
	}
	if order.Amount != 50000 {
		t.Fatalf("expected amount 50000, got %d", order.Amount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestWalletServiceConfirmTopUp_UpdatesBalanceAndWritesLedger(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now()
	walletRepo := repository.NewWalletRepository(db)
	topUpRepo := repository.NewTopUpRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	service := NewWalletService(db, walletRepo, topUpRepo, ledgerRepo, NewIdempotencyService(idempotencyRepo))

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO idempotency_keys (user_id, scope, idempotency_key, request_hash, status, locked_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		RETURNING id, user_id, scope, idempotency_key, request_hash, status, resource_type, resource_id, locked_at, last_seen_at, expires_at, created_at, updated_at`,
	)).
		WithArgs(1, model.IdempotencyScopeWalletTopUpConfirm, "topup-confirm-1", sqlmock.AnyArg(), model.IdempotencyStatusProcessing).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "scope", "idempotency_key", "request_hash", "status", "resource_type", "resource_id", "locked_at", "last_seen_at", "expires_at", "created_at", "updated_at",
		}).AddRow(
			12, 1, model.IdempotencyScopeWalletTopUpConfirm, "topup-confirm-1", "request-hash",
			model.IdempotencyStatusProcessing, sql.NullString{}, sql.NullInt64{}, sql.NullTime{Time: now, Valid: true}, now, sql.NullTime{}, now, now,
		))

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT t.id, t.wallet_id, t.reference_id, t.amount, t.status, t.payment_channel, t.external_reference, t.description, t.balance_before, t.balance_after,
			t.created_at::text, t.updated_at::text, t.confirmed_at::text, t.failed_at::text
		FROM top_up_orders t
		JOIN wallets w ON w.id = t.wallet_id
		WHERE w.user_id = $1 AND t.reference_id = $2
		FOR UPDATE OF t`,
	)).
		WithArgs(1, "TUP-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "wallet_id", "reference_id", "amount", "status", "payment_channel", "external_reference", "description", "balance_before", "balance_after", "created_at", "updated_at", "confirmed_at", "failed_at",
		}).AddRow(
			1, 1, "TUP-1", int64(50000), "pending", "manual", nil, "Top up pertama", nil, nil,
			"2026-05-19 09:30:00+00", "2026-05-19 09:30:00+00", nil, nil,
		))

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, user_id, balance, currency
		FROM wallets
		WHERE id = $1
		FOR UPDATE`,
	)).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance", "currency"}).
			AddRow(1, 1, int64(0), "IDR"))

	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE wallets SET balance = balance + $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
	)).
		WithArgs(int64(50000), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(
		`UPDATE top_up_orders
		SET status = 'success',
			external_reference = NULLIF($2, ''),
			description = CASE
				WHEN NULLIF($3, '') IS NULL THEN description
				ELSE $3
			END,
			balance_before = $4,
			balance_after = $5,
			confirmed_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING id, wallet_id, reference_id, amount, status, payment_channel, external_reference, description, balance_before, balance_after,
			created_at::text, updated_at::text, confirmed_at::text, failed_at::text`,
	)).
		WithArgs(1, "SIM-VA-1", "Simulated confirmation", int64(0), int64(50000)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "wallet_id", "reference_id", "amount", "status", "payment_channel", "external_reference", "description", "balance_before", "balance_after", "created_at", "updated_at", "confirmed_at", "failed_at",
		}).AddRow(
			1, 1, "TUP-1", int64(50000), "success", "manual", "SIM-VA-1", "Simulated confirmation", int64(0), int64(50000),
			"2026-05-19 09:30:00+00", "2026-05-19 09:31:00+00", "2026-05-19 09:31:00+00", nil,
		))

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO ledger_entries (
			wallet_id,
			reference_id,
			transaction_type,
			direction,
			amount,
			balance_before,
			balance_after,
			description,
			transfer_id,
			top_up_order_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10)
		RETURNING id`,
	)).
		WithArgs(1, "TUP-1", "top_up", "credit", int64(50000), int64(0), int64(50000), "Simulated confirmation", nil, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(101))

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE idempotency_keys
		SET status = $2,
			resource_type = $3,
			resource_id = $4,
			last_seen_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`,
	)).
		WithArgs(12, model.IdempotencyStatusCompleted, "top_up_order_confirm", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	order, err := service.ConfirmTopUp(context.Background(), 1, "TUP-1", "topup-confirm-1", model.ConfirmTopUpRequest{
		ExternalReference: "SIM-VA-1",
		Description:       "Simulated confirmation",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if order.Status != "success" {
		t.Fatalf("expected success order, got %s", order.Status)
	}
	if order.BalanceAfter == nil || *order.BalanceAfter != 50000 {
		t.Fatalf("expected balance_after 50000, got %+v", order.BalanceAfter)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func mustHashPayload(t *testing.T, payload any) string {
	t.Helper()

	hash, err := hashPayload(payload)
	if err != nil {
		t.Fatalf("failed to hash payload: %v", err)
	}

	return hash
}
