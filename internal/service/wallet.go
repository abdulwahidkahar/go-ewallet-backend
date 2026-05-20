package service

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
	"log/slog"
)

type WalletService struct {
	db                 *sql.DB
	walletRepo         *repository.WalletRepository
	topUpRepo          *repository.TopUpRepository
	ledgerRepo         *repository.LedgerRepository
	idempotencyService *IdempotencyService
}

func NewWalletService(
	db *sql.DB,
	walletRepo *repository.WalletRepository,
	topUpRepo *repository.TopUpRepository,
	ledgerRepo *repository.LedgerRepository,
	idempotencyService *IdempotencyService,
) *WalletService {
	return &WalletService{
		db:                 db,
		walletRepo:         walletRepo,
		topUpRepo:          topUpRepo,
		ledgerRepo:         ledgerRepo,
		idempotencyService: idempotencyService,
	}
}

func (s *WalletService) GetBalance(ctx context.Context, userID int) (int64, error) {
	balance, err := s.walletRepo.GetBalance(ctx, userID)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func (s *WalletService) GetWallet(ctx context.Context, userID int) (model.WalletResponse, error) {
	wallet, err := s.walletRepo.GetWalletByUserID(ctx, userID)
	if err != nil {
		return model.WalletResponse{}, err
	}

	return wallet, nil
}

func (s *WalletService) Transfer(ctx context.Context, fromUserID, toWalletID int, amount int64) (model.TransferResponse, error) {
	return s.TransferWithIdempotency(ctx, fromUserID, "", model.TransferRequest{
		ToWalletID: toWalletID,
		Amount:     amount,
	})
}

func (s *WalletService) TransferWithIdempotency(ctx context.Context, fromUserID int, idempotencyKey string, req model.TransferRequest) (model.TransferResponse, error) {
	amount := req.Amount
	if amount <= 0 {
		return model.TransferResponse{}, errors.New("amount must be greater than 0")
	}

	fromWallet, err := s.walletRepo.GetWalletByUserID(ctx, fromUserID)
	if err == sql.ErrNoRows {
		slog.Error("Sender wallet not found", "error", err, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, errors.New("sender wallet not found")
	}
	if err != nil {
		slog.Error("Failed to fetch sender wallet", "error", err, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, err
	}

	if fromWallet.ID == req.ToWalletID {
		slog.Error("Transfer to the same wallet is not allowed", "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, errors.New("cannot transfer to the same wallet")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("Failed to begin transaction for transfer", "error", err, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, err
	}
	defer tx.Rollback()

	var reservation model.IdempotencyReserveResult
	reservation, err = s.reserveIdempotency(ctx, tx, fromUserID, model.IdempotencyScopeWalletTransfer, idempotencyKey, req)
	if err != nil {
		return model.TransferResponse{}, err
	}

	if !reservation.IsNew {
		return s.resolveExistingTransfer(ctx, tx, reservation)
	}

	referenceID, err := generateReference("TRF")
	if err != nil {
		return model.TransferResponse{}, err
	}

	params := repository.TransferRequestParams{
		FromWalletID: fromWallet.ID,
		ToWalletID:   req.ToWalletID,
		Amount:       amount,
		ReferenceID:  referenceID,
		Description:  req.Description,
	}
	if reservation.Record.ID != 0 {
		params.IdempotencyKeyID = &reservation.Record.ID
	}
	params.CreatedByUserID = &fromUserID

	transfer, err := s.walletRepo.TransferWithMetadataTx(ctx, tx, params)
	if err != nil {
		if reservation.Record.ID != 0 {
			if failErr := s.failIdempotency(ctx, tx, reservation.Record.ID); failErr != nil {
				slog.Error("Failed to mark idempotency as failed", "error", failErr, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
			}
		}
		slog.Error("Transfer failed", "error", err, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, err
	}

	transferID := transfer.ID
	_, err = s.ledgerRepo.CreateTx(ctx, tx, model.LedgerEntry{
		WalletID:        transfer.FromWalletID,
		ReferenceID:     transfer.ReferenceID,
		TransactionType: "transfer",
		Direction:       "debit",
		Amount:          transfer.Amount,
		BalanceBefore:   transfer.SenderBalanceBefore,
		BalanceAfter:    transfer.SenderBalanceAfter,
		Description:     transfer.Description,
		TransferID:      &transferID,
	})
	if err != nil {
		return model.TransferResponse{}, err
	}

	_, err = s.ledgerRepo.CreateTx(ctx, tx, model.LedgerEntry{
		WalletID:        transfer.ToWalletID,
		ReferenceID:     transfer.ReferenceID,
		TransactionType: "transfer",
		Direction:       "credit",
		Amount:          transfer.Amount,
		BalanceBefore:   transfer.RecipientBalanceBefore,
		BalanceAfter:    transfer.RecipientBalanceAfter,
		Description:     transfer.Description,
		TransferID:      &transferID,
	})
	if err != nil {
		return model.TransferResponse{}, err
	}

	if reservation.Record.ID != 0 {
		if err := s.completeIdempotency(ctx, tx, reservation.Record.ID, "transfer", transfer.ID); err != nil {
			return model.TransferResponse{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit transfer transaction", "error", err, "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)
		return model.TransferResponse{}, err
	}

	slog.Info("Transfer successful", "from_user_id", fromUserID, "to_wallet_id", req.ToWalletID, "amount", amount)

	return transfer, nil
}

func (s *WalletService) GetHistoryTransfer(ctx context.Context, userID int, page, limit int) ([]model.TransferHistory, error) {
	history, err := s.walletRepo.TransferHistory(ctx, userID, page, limit)
	if err != nil {
		return nil, err
	}

	return history, nil
}

func (s *WalletService) CreateTopUp(ctx context.Context, userID int, idempotencyKey string, req model.CreateTopUpRequest) (model.TopUpOrder, error) {
	if req.Amount <= 0 {
		return model.TopUpOrder{}, errors.New("amount must be greater than 0")
	}

	if req.PaymentChannel == "" {
		req.PaymentChannel = "manual"
	}

	wallet, err := s.walletRepo.GetWalletByUserID(ctx, userID)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.TopUpOrder{}, err
	}
	defer tx.Rollback()

	reservation, err := s.reserveIdempotency(ctx, tx, userID, model.IdempotencyScopeWalletTopUpCreate, idempotencyKey, req)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if !reservation.IsNew {
		return s.resolveExistingTopUpCreate(ctx, tx, reservation)
	}

	referenceID, err := generateReference("TUP")
	if err != nil {
		return model.TopUpOrder{}, err
	}

	order, err := s.topUpRepo.CreateTx(ctx, tx, wallet.ID, reservation.Record.ID, referenceID, req)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if err := s.completeIdempotency(ctx, tx, reservation.Record.ID, "top_up_order", order.ID); err != nil {
		return model.TopUpOrder{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.TopUpOrder{}, err
	}

	return order, nil
}

func (s *WalletService) ConfirmTopUp(ctx context.Context, userID int, referenceID, idempotencyKey string, req model.ConfirmTopUpRequest) (model.TopUpOrder, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.TopUpOrder{}, err
	}
	defer tx.Rollback()

	reservation, err := s.reserveIdempotency(ctx, tx, userID, model.IdempotencyScopeWalletTopUpConfirm, idempotencyKey, struct {
		ReferenceID string `json:"reference_id"`
		model.ConfirmTopUpRequest
	}{
		ReferenceID:         referenceID,
		ConfirmTopUpRequest: req,
	})
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if !reservation.IsNew {
		return s.resolveExistingTopUpConfirm(ctx, tx, reservation)
	}

	order, err := s.topUpRepo.GetByReferenceIDForUserTx(ctx, tx, userID, referenceID)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if order.Status == "success" {
		if err := s.completeIdempotency(ctx, tx, reservation.Record.ID, "top_up_order_confirm", order.ID); err != nil {
			return model.TopUpOrder{}, err
		}
		if err := tx.Commit(); err != nil {
			return model.TopUpOrder{}, err
		}

		return order, nil
	}

	if order.Status == "failed" {
		if err := s.failIdempotency(ctx, tx, reservation.Record.ID); err != nil {
			return model.TopUpOrder{}, err
		}
		return model.TopUpOrder{}, errors.New("top up order already failed")
	}

	wallet, err := s.walletRepo.GetWalletByIDForUpdateTx(ctx, tx, order.WalletID)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	balanceBefore := wallet.Balance
	balanceAfter := wallet.Balance + order.Amount

	if err := s.walletRepo.UpdateBalanceByWalletIDTx(ctx, tx, order.WalletID, order.Amount); err != nil {
		return model.TopUpOrder{}, err
	}

	description := order.Description
	if req.Description != "" {
		description = req.Description
	}

	order, err = s.topUpRepo.MarkSuccessfulTx(ctx, tx, order.ID, req.ExternalReference, description, balanceBefore, balanceAfter)
	if err != nil {
		return model.TopUpOrder{}, err
	}

	topUpOrderID := order.ID
	_, err = s.ledgerRepo.CreateTx(ctx, tx, model.LedgerEntry{
		WalletID:        order.WalletID,
		ReferenceID:     order.ReferenceID,
		TransactionType: "top_up",
		Direction:       "credit",
		Amount:          order.Amount,
		BalanceBefore:   balanceBefore,
		BalanceAfter:    balanceAfter,
		Description:     description,
		TopUpOrderID:    &topUpOrderID,
	})
	if err != nil {
		return model.TopUpOrder{}, err
	}

	if err := s.completeIdempotency(ctx, tx, reservation.Record.ID, "top_up_order_confirm", order.ID); err != nil {
		return model.TopUpOrder{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.TopUpOrder{}, err
	}

	return order, nil
}

func (s *WalletService) GetTopUpOrders(ctx context.Context, userID int, page, limit int) ([]model.TopUpOrder, error) {
	return s.topUpRepo.History(ctx, userID, page, limit)
}

func (s *WalletService) GetLedgerEntries(ctx context.Context, userID int, page, limit int) ([]model.LedgerEntryHistory, error) {
	return s.ledgerRepo.History(ctx, userID, page, limit)
}

func (s *WalletService) reserveIdempotency(ctx context.Context, tx *sql.Tx, userID int, scope, key string, payload any) (model.IdempotencyReserveResult, error) {
	if s.idempotencyService == nil {
		return model.IdempotencyReserveResult{}, nil
	}

	return s.idempotencyService.Reserve(ctx, tx, userID, scope, key, payload)
}

func (s *WalletService) completeIdempotency(ctx context.Context, tx *sql.Tx, id int, resourceType string, resourceID int) error {
	if s.idempotencyService == nil || id == 0 {
		return nil
	}

	return s.idempotencyService.Complete(ctx, tx, id, resourceType, resourceID)
}

func (s *WalletService) failIdempotency(ctx context.Context, tx *sql.Tx, id int) error {
	if s.idempotencyService == nil || id == 0 {
		return nil
	}

	return s.idempotencyService.Fail(ctx, tx, id)
}

func (s *WalletService) resolveExistingTopUpCreate(ctx context.Context, tx *sql.Tx, reservation model.IdempotencyReserveResult) (model.TopUpOrder, error) {
	switch reservation.Record.Status {
	case model.IdempotencyStatusCompleted:
		if !reservation.Record.ResourceID.Valid {
			return model.TopUpOrder{}, sql.ErrNoRows
		}

		return s.topUpRepo.GetByID(ctx, tx, int(reservation.Record.ResourceID.Int64))
	case model.IdempotencyStatusFailed:
		return model.TopUpOrder{}, ErrIdempotencyRequestFailed
	default:
		return model.TopUpOrder{}, ErrIdempotencyRequestInProgress
	}
}

func (s *WalletService) resolveExistingTopUpConfirm(ctx context.Context, tx *sql.Tx, reservation model.IdempotencyReserveResult) (model.TopUpOrder, error) {
	switch reservation.Record.Status {
	case model.IdempotencyStatusCompleted:
		if !reservation.Record.ResourceID.Valid {
			return model.TopUpOrder{}, sql.ErrNoRows
		}

		return s.topUpRepo.GetByID(ctx, tx, int(reservation.Record.ResourceID.Int64))
	case model.IdempotencyStatusFailed:
		return model.TopUpOrder{}, ErrIdempotencyRequestFailed
	default:
		return model.TopUpOrder{}, ErrIdempotencyRequestInProgress
	}
}

func (s *WalletService) resolveExistingTransfer(ctx context.Context, tx *sql.Tx, reservation model.IdempotencyReserveResult) (model.TransferResponse, error) {
	switch reservation.Record.Status {
	case model.IdempotencyStatusCompleted:
		if !reservation.Record.ResourceID.Valid {
			return model.TransferResponse{}, sql.ErrNoRows
		}

		return s.walletRepo.GetTransferByID(ctx, tx, int(reservation.Record.ResourceID.Int64))
	case model.IdempotencyStatusFailed:
		return model.TransferResponse{}, ErrIdempotencyRequestFailed
	default:
		return model.TransferResponse{}, ErrIdempotencyRequestInProgress
	}
}
