package model

import (
	"database/sql"
	"time"
)

const (
	OutboxStatusPending   = "PENDING"
	OutboxStatusPublished = "PUBLISHED"
	OutboxStatusFailed    = "FAILED"
)

type OutboxEvent struct {
	ID          int64
	EventType   string
	Payload     []byte // JSON bytes
	Status      string
	RetryCount  int
	LastError   sql.NullString
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt sql.NullTime
}
