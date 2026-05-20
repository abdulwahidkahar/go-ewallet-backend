package model

type Transfer struct {
	ID           int    `json:"id"`
	FromWalletID int    `json:"from_wallet_id"`
	ToWalletID   int    `json:"to_wallet_id"`
	Amount       int64  `json:"amount"`
	ReferenceID  string `json:"reference_id"`
	Status       string `json:"status"`
	Description  string `json:"description,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type TransferRequest struct {
	ToWalletID  int    `json:"to_wallet_id"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
}

type TransferResponse struct {
	ID                     int    `json:"id"`
	FromWalletID           int    `json:"from_wallet_id"`
	ToWalletID             int    `json:"to_wallet_id"`
	Amount                 int64  `json:"amount"`
	ReferenceID            string `json:"reference_id"`
	Status                 string `json:"status"`
	Description            string `json:"description,omitempty"`
	SenderBalanceBefore    int64  `json:"sender_balance_before"`
	SenderBalanceAfter     int64  `json:"sender_balance_after"`
	RecipientBalanceBefore int64  `json:"recipient_balance_before"`
	RecipientBalanceAfter  int64  `json:"recipient_balance_after"`
	CreatedAt              string `json:"created_at"`
}

type TransferHistory struct {
	ID           int    `json:"id"`
	FromWalletID int    `json:"from_wallet_id"`
	ToWalletID   int    `json:"to_wallet_id"`
	Amount       int64  `json:"amount"`
	ReferenceID  string `json:"reference_id"`
	Status       string `json:"status"`
	Description  string `json:"description,omitempty"`
	CreatedAt    string `json:"created_at"`
}
