package repository

import (
	"context"
	"database/sql"
	"go-ewallet-backend/internal/model"
)

type LedgerRepository struct {
	db *sql.DB
}

func NewLedgerRepository(db *sql.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) CreateTx(ctx context.Context, tx *sql.Tx, entry model.LedgerEntry) (int, error) {
	var id int
	var transferID any
	var topUpOrderID any

	if entry.TransferID != nil {
		transferID = *entry.TransferID
	}
	if entry.TopUpOrderID != nil {
		topUpOrderID = *entry.TopUpOrderID
	}

	err := tx.QueryRowContext(
		ctx,
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
		entry.WalletID,
		entry.ReferenceID,
		entry.TransactionType,
		entry.Direction,
		entry.Amount,
		entry.BalanceBefore,
		entry.BalanceAfter,
		entry.Description,
		transferID,
		topUpOrderID,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
