package model

type CreateTopUpRequest struct {
	Amount         int64  `json:"amount"`
	PaymentChannel string `json:"payment_channel"`
	Description    string `json:"description"`
}

type ConfirmTopUpRequest struct {
	ExternalReference string `json:"external_reference"`
	Description       string `json:"description"`
}

type TopUpOrder struct {
	ID                int     `json:"id"`
	WalletID          int     `json:"wallet_id"`
	ReferenceID       string  `json:"reference_id"`
	Amount            int64   `json:"amount"`
	Status            string  `json:"status"`
	PaymentChannel    string  `json:"payment_channel"`
	ExternalReference *string `json:"external_reference,omitempty"`
	Description       string  `json:"description,omitempty"`
	BalanceBefore     *int64  `json:"balance_before,omitempty"`
	BalanceAfter      *int64  `json:"balance_after,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	ConfirmedAt       *string `json:"confirmed_at,omitempty"`
	FailedAt          *string `json:"failed_at,omitempty"`
}

type LedgerEntry struct {
	ID              int
	WalletID        int
	ReferenceID     string
	TransactionType string
	Direction       string
	Amount          int64
	BalanceBefore   int64
	BalanceAfter    int64
	Description     string
	TransferID      *int
	TopUpOrderID    *int
	CreatedAt       string
}
