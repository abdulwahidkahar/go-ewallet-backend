package handler

import (
	"auth-api/model"
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	db *sql.DB
}

func NewWalletHandler(db *sql.DB) *WalletHandler {
	return &WalletHandler{db: db}
}

func CreateWallet(ctx context.Context, tx *sql.Tx, userID int) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO wallets (user_id, balance, currency) VALUES ($1, 0, 'IDR')",
		userID,
	)
	return err
}

func (wh *WalletHandler) TopUp(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))

	var req model.WalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	tx, err := wh.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(c.Request.Context(),
		"UPDATE wallets SET balance = balance + $1 WHERE user_id = $2",
		req.Amount, userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating wallet balance"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error committing transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Wallet topped up successfully", "top_up": req.Amount})
}

func (wh *WalletHandler) GetBalance(c *gin.Context) {

	userID := int(c.MustGet("id").(float64))

	query := "SELECT id, user_id, balance, currency, created_at, updated_at FROM wallets WHERE user_id = $1"

	var wallet model.WalletResponse

	err := wh.db.QueryRowContext(c.Request.Context(), query, userID).
		Scan(&wallet.ID, &wallet.UserID, &wallet.Balance, &wallet.Currency, &wallet.CreatedAt, &wallet.UpdatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Balance not found or user does not exist"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching wallet balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"wallet": wallet})

}

func (wh *WalletHandler) Transfer(c *gin.Context) {

	userID := int(c.MustGet("id").(float64))

	var req model.TransferRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	tx, err := wh.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	defer tx.Rollback()
	var fromWallet model.Wallet
	err = tx.QueryRowContext(c.Request.Context(),
		"SELECT id, user_id, balance, currency FROM wallets WHERE user_id = $1 FOR UPDATE",
		userID,
	).Scan(&fromWallet.ID, &fromWallet.UserID, &fromWallet.Balance, &fromWallet.Currency)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sender wallet not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching sender wallet"})
		return
	}

	if req.Amount > fromWallet.Balance {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient balance"})
		return
	}

	var toWallet model.Wallet
	err = tx.QueryRowContext(c.Request.Context(),
		"SELECT id, user_id, balance, currency FROM wallets WHERE id = $1 FOR UPDATE",
		req.ToWalletID,
	).Scan(&toWallet.ID, &toWallet.UserID, &toWallet.Balance, &toWallet.Currency)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Recipient wallet not found"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching recipient wallet"})
		return
	}

	_, err = tx.ExecContext(c.Request.Context(),
		"UPDATE wallets SET balance = balance - $1 WHERE user_id = $2",
		req.Amount, userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating sender wallet"})
		return
	}

	_, err = tx.ExecContext(c.Request.Context(),
		"UPDATE wallets SET balance = balance + $1 WHERE id = $2",
		req.Amount, req.ToWalletID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating recipient wallet"})
		return
	}

	var transferID int
	err = tx.QueryRowContext(c.Request.Context(),
		"INSERT INTO transfers (from_wallet_id, to_wallet_id, amount) VALUES ($1, $2, $3) RETURNING id",
		fromWallet.ID, toWallet.ID, req.Amount,
	).Scan(&transferID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error recording transfer"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error committing transaction"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Transfer successful", "transfer": model.TransferResponse{
		ID:           transferID,
		FromWalletID: fromWallet.ID,
		ToWalletID:   toWallet.ID,
		Amount:       req.Amount,
	}})
}

func (wh *WalletHandler) GetHistoryTransfer(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))

	query := `
		SELECT t.id, t.from_wallet_id, t.to_wallet_id, t.amount, t.created_at
		FROM transfers t
		JOIN wallets w ON (t.from_wallet_id = w.id OR t.to_wallet_id = w.id)
		WHERE w.user_id = $1
		ORDER BY t.created_at DESC
	`

	rows, err := wh.db.QueryContext(c.Request.Context(), query, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching transfer history"})
		return
	}
	defer rows.Close()

	var transfers []model.TransferResponse
	for rows.Next() {
		var transfer model.TransferResponse
		err = rows.Scan(&transfer.ID, &transfer.FromWalletID, &transfer.ToWalletID, &transfer.Amount, &transfer.CreatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error scanning transfer data"})
			return
		}
		transfers = append(transfers, transfer)
	}

	c.JSON(http.StatusOK, gin.H{"transfers": transfers})
}
