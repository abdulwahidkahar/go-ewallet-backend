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

func (r *LedgerRepository) History(ctx context.Context, userID int, page, limit int) ([]model.LedgerEntryHistory, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT l.id, l.wallet_id, l.reference_id, l.transaction_type, l.direction, l.amount, l.balance_before, l.balance_after,
			l.description, l.transfer_id, l.top_up_order_id, l.created_at::text
		FROM ledger_entries l
		JOIN wallets w ON w.id = l.wallet_id
		WHERE w.user_id = $1
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $2 OFFSET $3`,
		userID,
		limit,
		(page-1)*limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.LedgerEntryHistory
	for rows.Next() {
		var entry model.LedgerEntryHistory
		var description sql.NullString
		var transferID sql.NullInt64
		var topUpOrderID sql.NullInt64

		if err := rows.Scan(
			&entry.ID,
			&entry.WalletID,
			&entry.ReferenceID,
			&entry.TransactionType,
			&entry.Direction,
			&entry.Amount,
			&entry.BalanceBefore,
			&entry.BalanceAfter,
			&description,
			&transferID,
			&topUpOrderID,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}

		if description.Valid {
			entry.Description = description.String
		}
		if transferID.Valid {
			value := int(transferID.Int64)
			entry.TransferID = &value
		}
		if topUpOrderID.Valid {
			value := int(topUpOrderID.Int64)
			entry.TopUpOrderID = &value
		}

		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}
