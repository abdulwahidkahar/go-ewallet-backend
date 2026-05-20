package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestWalletRepositoryTransferWithMetadataTx_ReturnsErrorForInsufficientBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewWalletRepository(db)

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT id, user_id, balance, currency
		FROM wallets
		WHERE id = $1 OR id = $2
		ORDER BY id
		FOR UPDATE`,
	)).
		WithArgs(1, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "balance", "currency"}).
			AddRow(1, 1, int64(10000), "IDR").
			AddRow(2, 2, int64(5000), "IDR"))

	_, err = repo.TransferWithMetadataTx(context.Background(), tx, TransferRequestParams{
		FromWalletID: 1,
		ToWalletID:   2,
		Amount:       20000,
		ReferenceID:  "TRF-test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "insufficient balance" {
		t.Fatalf("expected insufficient balance error, got %v", err)
	}

	mock.ExpectRollback()
	if err := tx.Rollback(); err != nil {
		t.Fatalf("failed to rollback tx: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
