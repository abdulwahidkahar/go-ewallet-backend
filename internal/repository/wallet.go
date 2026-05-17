package repository

import (
	"auth-api/internal/model"
	"context"
	"database/sql"
	"errors"
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

func (r *WalletRepository) CreateTopUpHistoryTx(ctx context.Context, tx *sql.Tx, userID int, amount int64) error {
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO top_up_history (wallet_id, amount)
		SELECT id, $1 FROM wallets WHERE user_id = $2`,
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

func (r *WalletRepository) TransferTx(ctx context.Context, tx *sql.Tx, fromUserID, toWalletID int, amount int64) (model.TransferResponse, error) {
	var fromWallet model.Wallet
	err := tx.QueryRowContext(ctx,
		"SELECT id, user_id, balance, currency FROM wallets WHERE user_id = $1",
		fromUserID,
	).Scan(&fromWallet.ID, &fromWallet.UserID, &fromWallet.Balance, &fromWallet.Currency)
	if err == sql.ErrNoRows {
		return model.TransferResponse{}, errors.New("sender wallet not found")
	}
	if err != nil {
		return model.TransferResponse{}, err
	}

	if fromWallet.ID == toWalletID {
		return model.TransferResponse{}, errors.New("cannot transfer to the same wallet")
	}

	firstWalletID, secondWalletID := fromWallet.ID, toWalletID
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

	fromWallet, ok := lockedWallets[fromWallet.ID]
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
	err = tx.QueryRowContext(ctx,
		"INSERT INTO transfers (from_wallet_id, to_wallet_id, amount) VALUES ($1, $2, $3) RETURNING id, created_at::text",
		fromWallet.ID, toWallet.ID, amount,
	).Scan(&transferID, &createdAt)
	if err != nil {
		return model.TransferResponse{}, err
	}

	return model.TransferResponse{
		ID:           transferID,
		FromWalletID: fromWallet.ID,
		ToWalletID:   toWallet.ID,
		Amount:       amount,
		CreatedAt:    createdAt,
	}, nil
}

func (r *WalletRepository) TransferHistory(ctx context.Context, userID int) ([]model.TransferHistory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.from_wallet_id, t.to_wallet_id, t.amount, t.created_at
		FROM transfers t
		JOIN wallets w ON (t.from_wallet_id = w.id OR t.to_wallet_id = w.id)
		WHERE w.user_id = $1
		ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.TransferHistory
	for rows.Next() {
		var transfer model.TransferHistory
		err := rows.Scan(&transfer.ID, &transfer.FromWalletID, &transfer.ToWalletID, &transfer.Amount, &transfer.CreatedAt)
		if err != nil {
			return nil, err
		}
		history = append(history, transfer)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}

func (r *WalletRepository) TopUpHistory(ctx context.Context, userID int) ([]model.TopUpHistory, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.wallet_id, t.amount, t.created_at
		FROM top_up_history t
		JOIN wallets w ON t.wallet_id = w.id
		WHERE w.user_id = $1
		ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.TopUpHistory
	for rows.Next() {
		var topUp model.TopUpHistory
		err := rows.Scan(&topUp.ID, &topUp.WalletID, &topUp.Amount, &topUp.CreatedAt)
		if err != nil {
			return nil, err
		}
		history = append(history, topUp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}
