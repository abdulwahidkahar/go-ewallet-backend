package model

type Wallet struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Balance   int64  `json:"balance"`
	Currency  string `json:"currency"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type WalletBalanceSnapshot struct {
	ID       int
	UserID   int
	Balance  int64
	Currency string
}

type WalletRequest struct {
	Amount int64 `json:"amount"`
}

type WalletResponse struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Balance   int64  `json:"balance"`
	Currency  string `json:"currency"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
