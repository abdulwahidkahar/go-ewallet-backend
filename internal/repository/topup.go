package repository

import (
	"context"
	"database/sql"
	"go-ewallet-backend/internal/model"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type TopUpRepository struct {
	db *sql.DB
}

func NewTopUpRepository(db *sql.DB) *TopUpRepository {
	return &TopUpRepository{db: db}
}

func (r *TopUpRepository) CreateTx(ctx context.Context, tx *sql.Tx, walletID, idempotencyKeyID int, referenceID string, req model.CreateTopUpRequest) (model.TopUpOrder, error) {
	return r.scanTopUpOrder(tx.QueryRowContext(
		ctx,
		`INSERT INTO top_up_orders (wallet_id, reference_id, idempotency_key_id, amount, status, payment_channel, description)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id, wallet_id, reference_id, amount, status, payment_channel, external_reference, description, balance_before, balance_after,
			created_at::text, updated_at::text, confirmed_at::text, failed_at::text`,
		walletID,
		referenceID,
		idempotencyKeyID,
		req.Amount,
		req.PaymentChannel,
		req.Description,
	))
}

func (r *TopUpRepository) GetByID(ctx context.Context, runner queryRowExecer, id int) (model.TopUpOrder, error) {
	runner = r.resolveRunner(runner)

	return r.scanTopUpOrder(runner.QueryRowContext(
		ctx,
		`SELECT id, wallet_id, reference_id, amount, status, payment_channel, external_reference, description, balance_before, balance_after,
			created_at::text, updated_at::text, confirmed_at::text, failed_at::text
		FROM top_up_orders
		WHERE id = $1`,
		id,
	))
}

func (r *TopUpRepository) GetByReferenceIDForUserTx(ctx context.Context, tx *sql.Tx, userID int, referenceID string) (model.TopUpOrder, error) {
	return r.scanTopUpOrder(tx.QueryRowContext(
		ctx,
		`SELECT t.id, t.wallet_id, t.reference_id, t.amount, t.status, t.payment_channel, t.external_reference, t.description, t.balance_before, t.balance_after,
			t.created_at::text, t.updated_at::text, t.confirmed_at::text, t.failed_at::text
		FROM top_up_orders t
		JOIN wallets w ON w.id = t.wallet_id
		WHERE w.user_id = $1 AND t.reference_id = $2
		FOR UPDATE OF t`,
		userID,
		referenceID,
	))
}

func (r *TopUpRepository) MarkSuccessfulTx(ctx context.Context, tx *sql.Tx, id int, externalReference, description string, balanceBefore, balanceAfter int64) (model.TopUpOrder, error) {
	return r.scanTopUpOrder(tx.QueryRowContext(
		ctx,
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
		id,
		externalReference,
		description,
		balanceBefore,
		balanceAfter,
	))
}

func (r *TopUpRepository) History(ctx context.Context, userID int, page, limit int) ([]model.TopUpOrder, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT t.id, t.wallet_id, t.reference_id, t.amount, t.status, t.payment_channel, t.external_reference, t.description, t.balance_before, t.balance_after,
			t.created_at::text, t.updated_at::text, t.confirmed_at::text, t.failed_at::text
		FROM top_up_orders t
		JOIN wallets w ON w.id = t.wallet_id
		WHERE w.user_id = $1
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3`,
		userID,
		limit,
		(page-1)*limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.TopUpOrder
	for rows.Next() {
		order, scanErr := r.scanTopUpOrder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *TopUpRepository) scanTopUpOrder(scanner rowScanner) (model.TopUpOrder, error) {
	var order model.TopUpOrder
	var externalReference sql.NullString
	var description sql.NullString
	var balanceBefore sql.NullInt64
	var balanceAfter sql.NullInt64
	var confirmedAt sql.NullString
	var failedAt sql.NullString

	err := scanner.Scan(
		&order.ID,
		&order.WalletID,
		&order.ReferenceID,
		&order.Amount,
		&order.Status,
		&order.PaymentChannel,
		&externalReference,
		&description,
		&balanceBefore,
		&balanceAfter,
		&order.CreatedAt,
		&order.UpdatedAt,
		&confirmedAt,
		&failedAt,
	)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if externalReference.Valid {
		value := externalReference.String
		order.ExternalReference = &value
	}
	if description.Valid {
		order.Description = description.String
	}
	if balanceBefore.Valid {
		value := balanceBefore.Int64
		order.BalanceBefore = &value
	}
	if balanceAfter.Valid {
		value := balanceAfter.Int64
		order.BalanceAfter = &value
	}
	if confirmedAt.Valid {
		value := confirmedAt.String
		order.ConfirmedAt = &value
	}
	if failedAt.Valid {
		value := failedAt.String
		order.FailedAt = &value
	}

	return order, nil
}

func (r *TopUpRepository) resolveRunner(runner queryRowExecer) queryRowExecer {
	if runner != nil {
		return runner
	}

	return r.db
}
