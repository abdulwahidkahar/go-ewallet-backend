package model

import (
	"database/sql"
	"time"
)

const (
	IdempotencyStatusProcessing = "processing"
	IdempotencyStatusCompleted  = "completed"
	IdempotencyStatusFailed     = "failed"
)

const (
	IdempotencyScopeWalletTransfer     = "wallet.transfer"
	IdempotencyScopeWalletTopUpCreate  = "wallet.topup.create"
	IdempotencyScopeWalletTopUpConfirm = "wallet.topup.confirm"
)

type IdempotencyKey struct {
	ID             int
	UserID         int
	Scope          string
	IdempotencyKey string
	RequestHash    string
	Status         string
	ResourceType   sql.NullString
	ResourceID     sql.NullInt64
	LockedAt       sql.NullTime
	LastSeenAt     time.Time
	ExpiresAt      sql.NullTime
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type IdempotencyReserveResult struct {
	Record IdempotencyKey
	IsNew  bool
}
