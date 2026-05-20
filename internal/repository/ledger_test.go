package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLedgerRepositoryHistory_ReturnsLedgerEntriesForUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewLedgerRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT l.id, l.wallet_id, l.reference_id, l.transaction_type, l.direction, l.amount, l.balance_before, l.balance_after,
			l.description, l.transfer_id, l.top_up_order_id, l.created_at::text
		FROM ledger_entries l
		JOIN wallets w ON w.id = l.wallet_id
		WHERE w.user_id = $1
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $2 OFFSET $3`,
	)).
		WithArgs(1, 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "wallet_id", "reference_id", "transaction_type", "direction", "amount", "balance_before", "balance_after",
			"description", "transfer_id", "top_up_order_id", "created_at",
		}).AddRow(
			101, 1, "TRF-1", "transfer", "debit", int64(20000), int64(50000), int64(30000),
			"transfer from test", 10, nil, "2026-05-20 10:00:00+00",
		))

	entries, err := repo.History(context.Background(), 1, 1, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(entries))
	}
	if entries[0].ReferenceID != "TRF-1" {
		t.Fatalf("expected reference id TRF-1, got %s", entries[0].ReferenceID)
	}
	if entries[0].TransferID == nil || *entries[0].TransferID != 10 {
		t.Fatalf("expected transfer id 10, got %+v", entries[0].TransferID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
