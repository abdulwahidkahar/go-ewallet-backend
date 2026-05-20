package repository

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"
	"strconv"
	"time"
)

type WalletRepository struct {
	db *sql.DB
}

func NewWalletRepository(db *sql.DB) *WalletRepository {
	return &WalletRepository{db: db}
}

func (r *WalletRepository) CreateTx(ctx context.Context, tx *sql.Tx, userID int) (int, error) {
	var walletID int

	err := tx.QueryRowContext(
		ctx,
		"INSERT INTO wallets (user_id, balance) VALUES ($1, 0) RETURNING id",
		userID,
	).Scan(&walletID)
	if err != nil {
		return 0, err
	}

	return walletID, nil
}

func (r *WalletRepository) GetBalance(ctx context.Context, userID int) (int64, error) {
	var balance int64
	err := r.db.QueryRowContext(
		ctx,
		"SELECT balance FROM wallets WHERE user_id = $1",
		userID,
	).Scan(&balance)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return balance, nil
}

func (r *WalletRepository) GetWalletByUserID(ctx context.Context, userID int) (model.WalletResponse, error) {
	var wallet model.WalletResponse

	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, user_id, balance, currency, created_at, updated_at FROM wallets WHERE user_id = $1",
		userID,
	).Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Currency, &wallet.CreatedAt, &wallet.UpdatedAt)
	if err != nil {
		return model.WalletResponse{}, err
	}

	return wallet, nil
}

func (r *WalletRepository) UpdateBalanceTx(ctx context.Context, tx *sql.Tx, userID int, amount int64) error {
	result, err := tx.ExecContext(
		ctx,
		"UPDATE wallets SET balance = balance + $1 WHERE user_id = $2",
		amount,
		userID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *WalletRepository) GetWalletByIDForUpdateTx(ctx context.Context, tx *sql.Tx, walletID int) (model.WalletBalanceSnapshot, error) {
	var wallet model.WalletBalanceSnapshot

	err := tx.QueryRowContext(
		ctx,
		`SELECT id, user_id, balance, currency
		FROM wallets
		WHERE id = $1
		FOR UPDATE`,
		walletID,
	).Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Currency)
	if err != nil {
		return model.WalletBalanceSnapshot{}, err
	}

	return wallet, nil
}

func (r *WalletRepository) UpdateBalanceByWalletIDTx(ctx context.Context, tx *sql.Tx, walletID int, amount int64) error {
	result, err := tx.ExecContext(
		ctx,
		"UPDATE wallets SET balance = balance + $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		amount,
		walletID,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *WalletRepository) TransferTx(ctx context.Context, tx *sql.Tx, fromWalletID, toWalletID int, amount int64) (model.TransferResponse, error) {
	return r.TransferWithMetadataTx(ctx, tx, TransferRequestParams{
		FromWalletID:     fromWalletID,
		ToWalletID:       toWalletID,
		Amount:           amount,
		ReferenceID:      "",
		Description:      "",
		IdempotencyKeyID: nil,
		CreatedByUserID:  nil,
	})
}

type TransferRequestParams struct {
	FromWalletID     int
	ToWalletID       int
	Amount           int64
	ReferenceID      string
	Description      string
	IdempotencyKeyID *int
	CreatedByUserID  *int
}

func (r *WalletRepository) TransferWithMetadataTx(ctx context.Context, tx *sql.Tx, params TransferRequestParams) (model.TransferResponse, error) {
	fromWalletID := params.FromWalletID
	toWalletID := params.ToWalletID
	amount := params.Amount

	if fromWalletID == toWalletID {
		return model.TransferResponse{}, errors.New("cannot transfer to the same wallet")
	}

	firstWalletID, secondWalletID := fromWalletID, toWalletID
	if firstWalletID > secondWalletID {
		firstWalletID, secondWalletID = secondWalletID, firstWalletID
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, user_id, balance, currency
		FROM wallets
		WHERE id = $1 OR id = $2
		ORDER BY id
		FOR UPDATE`,
		firstWalletID, secondWalletID,
	)
	if err != nil {
		return model.TransferResponse{}, err
	}
	defer rows.Close()

	lockedWallets := make(map[int]model.Wallet, 2)
	for rows.Next() {
		var wallet model.Wallet
		if err := rows.Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Currency); err != nil {
			return model.TransferResponse{}, err
		}
		lockedWallets[wallet.ID] = wallet
	}
	if err := rows.Err(); err != nil {
		return model.TransferResponse{}, err
	}

	fromWallet, ok := lockedWallets[fromWalletID]
	if !ok {
		return model.TransferResponse{}, errors.New("sender wallet not found")
	}

	toWallet, ok := lockedWallets[toWalletID]
	if !ok {
		return model.TransferResponse{}, errors.New("recipient wallet not found")
	}

	if amount > fromWallet.Balance {
		return model.TransferResponse{}, errors.New("insufficient balance")
	}

	senderBalanceBefore := fromWallet.Balance
	senderBalanceAfter := fromWallet.Balance - amount
	recipientBalanceBefore := toWallet.Balance
	recipientBalanceAfter := toWallet.Balance + amount

	_, err = tx.ExecContext(ctx,
		"UPDATE wallets SET balance = balance - $1 WHERE id = $2",
		amount, fromWallet.ID,
	)
	if err != nil {
		return model.TransferResponse{}, err
	}

	_, err = tx.ExecContext(ctx,
		"UPDATE wallets SET balance = balance + $1 WHERE id = $2",
		amount, toWallet.ID,
	)
	if err != nil {
		return model.TransferResponse{}, err
	}

	var transferID int
	var createdAt string
	var idempotencyKeyID any
	var createdByUserID any

	if params.IdempotencyKeyID != nil {
		idempotencyKeyID = *params.IdempotencyKeyID
	}
	if params.CreatedByUserID != nil {
		createdByUserID = *params.CreatedByUserID
	}

	referenceID := params.ReferenceID
	if referenceID == "" {
		referenceID = "legacy-transfer-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO transfers (
			from_wallet_id,
			to_wallet_id,
			amount,
			reference_id,
			status,
			description,
			idempotency_key_id,
			sender_balance_before,
			sender_balance_after,
			recipient_balance_before,
			recipient_balance_after,
			created_by_user_id
		) VALUES ($1, $2, $3, $4, 'success', NULLIF($5, ''), $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at::text`,
		fromWallet.ID,
		toWallet.ID,
		amount,
		referenceID,
		params.Description,
		idempotencyKeyID,
		senderBalanceBefore,
		senderBalanceAfter,
		recipientBalanceBefore,
		recipientBalanceAfter,
		createdByUserID,
	).Scan(&transferID, &createdAt)
	if err != nil {
		return model.TransferResponse{}, err
	}

	return model.TransferResponse{
		ID:                     transferID,
		FromWalletID:           fromWallet.ID,
		ToWalletID:             toWallet.ID,
		Amount:                 amount,
		ReferenceID:            referenceID,
		Status:                 "success",
		Description:            params.Description,
		SenderBalanceBefore:    senderBalanceBefore,
		SenderBalanceAfter:     senderBalanceAfter,
		RecipientBalanceBefore: recipientBalanceBefore,
		RecipientBalanceAfter:  recipientBalanceAfter,
		CreatedAt:              createdAt,
	}, nil
}

func (r *WalletRepository) GetTransferByID(ctx context.Context, runner queryRowExecer, id int) (model.TransferResponse, error) {
	runner = resolveQueryRunner(r, runner)

	var transfer model.TransferResponse
	var description sql.NullString

	err := runner.QueryRowContext(
		ctx,
		`SELECT id, from_wallet_id, to_wallet_id, amount, reference_id, status, description,
			sender_balance_before, sender_balance_after, recipient_balance_before, recipient_balance_after, created_at::text
		FROM transfers
		WHERE id = $1`,
		id,
	).Scan(
		&transfer.ID,
		&transfer.FromWalletID,
		&transfer.ToWalletID,
		&transfer.Amount,
		&transfer.ReferenceID,
		&transfer.Status,
		&description,
		&transfer.SenderBalanceBefore,
		&transfer.SenderBalanceAfter,
		&transfer.RecipientBalanceBefore,
		&transfer.RecipientBalanceAfter,
		&transfer.CreatedAt,
	)
	if err != nil {
		return model.TransferResponse{}, err
	}

	if description.Valid {
		transfer.Description = description.String
	}

	return transfer, nil
}

func (r *WalletRepository) TransferHistory(ctx context.Context, userID int, page, limit int) ([]model.TransferHistory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.from_wallet_id, t.to_wallet_id, t.amount, t.reference_id, t.status, t.description, t.created_at
		FROM transfers t
		JOIN wallets w ON (t.from_wallet_id = w.id OR t.to_wallet_id = w.id)
		WHERE w.user_id = $1
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.TransferHistory
	for rows.Next() {
		var transfer model.TransferHistory
		var description sql.NullString
		err := rows.Scan(&transfer.ID, &transfer.FromWalletID, &transfer.ToWalletID, &transfer.Amount, &transfer.ReferenceID, &transfer.Status, &description, &transfer.CreatedAt)
		if err != nil {
			return nil, err
		}
		if description.Valid {
			transfer.Description = description.String
		}
		history = append(history, transfer)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}

func resolveQueryRunner(walletRepo *WalletRepository, runner queryRowExecer) queryRowExecer {
	if runner != nil {
		return runner
	}

	return walletRepo.db
}
