package handler

import (
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	walletService *service.WalletService
}

func NewWalletHandler(walletService *service.WalletService) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

func (wh *WalletHandler) CreateTopUp(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))
	idempotencyKey := c.GetHeader("Idempotency-Key")

	var req model.CreateTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	order, err := wh.walletService.CreateTopUp(c.Request.Context(), userID, idempotencyKey, req)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Top up order created successfully",
		"top_up":  order,
	})
}

func (wh *WalletHandler) ConfirmTopUp(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))
	idempotencyKey := c.GetHeader("Idempotency-Key")
	referenceID := c.Param("reference_id")

	var req model.ConfirmTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	order, err := wh.walletService.ConfirmTopUp(c.Request.Context(), userID, referenceID, idempotencyKey, req)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Top up confirmed successfully",
		"top_up":  order,
	})
}

func (wh *WalletHandler) GetBalance(c *gin.Context) {

	userID := int(c.MustGet("id").(float64))

	wallet, err := wh.walletService.GetWallet(c.Request.Context(), userID)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{"wallet": wallet})

}

func (wh *WalletHandler) Transfer(c *gin.Context) {

	userID := int(c.MustGet("id").(float64))
	idempotencyKey := c.GetHeader("Idempotency-Key")

	var req model.TransferRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	transfer, err := wh.walletService.TransferWithIdempotency(c.Request.Context(), userID, idempotencyKey, req)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Transfer successful", "transfer": transfer})
}

func (wh *WalletHandler) GetHistoryTransfer(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))

	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")

	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter"})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	transfers, err := wh.walletService.GetHistoryTransfer(c.Request.Context(), userID, pageInt, limitInt)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.Header("X-Page", page)
	c.Header("X-Limit", limit)

	c.JSON(http.StatusOK, gin.H{"transfers": transfers})
}

func (wh *WalletHandler) GetTopUpOrders(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))

	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")

	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter"})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	topUps, err := wh.walletService.GetTopUpOrders(c.Request.Context(), userID, pageInt, limitInt)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{"top_ups": topUps})
}

func (wh *WalletHandler) GetLedgerEntries(c *gin.Context) {
	userID := int(c.MustGet("id").(float64))

	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")

	pageInt, err := strconv.Atoi(page)
	if err != nil || pageInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page parameter"})
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil || limitInt <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	entries, err := wh.walletService.GetLedgerEntries(c.Request.Context(), userID, pageInt, limitInt)
	if err != nil {
		statusCode, message := mapWalletError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ledger_entries": entries})
}

func mapWalletError(err error) (int, string) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound, "Wallet not found"
	case err.Error() == "amount must be greater than 0":
		return http.StatusBadRequest, err.Error()
	case err.Error() == "cannot transfer to the same wallet":
		return http.StatusBadRequest, err.Error()
	case err.Error() == "insufficient balance":
		return http.StatusBadRequest, "Insufficient balance"
	case err.Error() == "sender wallet not found":
		return http.StatusNotFound, "Sender wallet not found"
	case err.Error() == "recipient wallet not found":
		return http.StatusNotFound, "Recipient wallet not found"
	case errors.Is(err, service.ErrIdempotencyKeyRequired):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrIdempotencyKeyConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrIdempotencyRequestInProgress):
		return http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrIdempotencyRequestFailed):
		return http.StatusConflict, err.Error()
	case err.Error() == "top up order already failed":
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
