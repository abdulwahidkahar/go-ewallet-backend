package service

import (
	"context"
	"database/sql"
	"go-ewallet-backend/internal/repository"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultPublishInterval = 1 * time.Second
	defaultBatchSize       = 100
	retryWarningThreshold  = 10
	redisStreamName        = "notification:events"
)

type OutboxPublisher struct {
	db          *sql.DB
	redisClient *redis.Client
	outboxRepo  *repository.OutboxRepository
	interval    time.Duration
	batchSize   int
}

func NewOutboxPublisher(
	db *sql.DB,
	redisClient *redis.Client,
	outboxRepo *repository.OutboxRepository,
) *OutboxPublisher {
	return &OutboxPublisher{
		db:          db,
		redisClient: redisClient,
		outboxRepo:  outboxRepo,
		interval:    defaultPublishInterval,
		batchSize:   defaultBatchSize,
	}
}

// Start begins the polling loop. It blocks until ctx is cancelled.
func (p *OutboxPublisher) Start(ctx context.Context) {
	slog.Info("Outbox publisher started", "interval", p.interval, "batch_size", p.batchSize)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Outbox publisher shutting down")
			return
		case <-ticker.C:
			if err := p.publishBatch(ctx); err != nil {
				slog.Error("Outbox publisher batch failed", "error", err)
			}
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	events, err := p.outboxRepo.GetPendingForPublish(ctx, tx, p.batchSize)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	publishedCount := 0

	for _, event := range events {
		err := p.redisClient.XAdd(ctx, &redis.XAddArgs{
			Stream: redisStreamName,
			Values: map[string]interface{}{
				"payload": event.Payload,
			},
		}).Err()

		if err != nil {
			nextRetry := event.RetryCount + 1

			logLevel := slog.LevelError
			if nextRetry >= retryWarningThreshold {
				logLevel = slog.LevelWarn
			}

			slog.Log(ctx, logLevel, "Failed to publish outbox event to Redis",
				"event_id", event.ID,
				"event_type", event.EventType,
				"retry_count", nextRetry,
				"error", err,
			)

			if nextRetry >= retryWarningThreshold && nextRetry%retryWarningThreshold == 0 {
				slog.Warn("Outbox event stuck — Redis mungkin masih down, tetap retry dengan backoff",
					"event_id", event.ID,
					"retry_count", nextRetry,
				)
			}

			if markErr := p.outboxRepo.RecordRetry(ctx, tx, event.ID, err.Error()); markErr != nil {
				slog.Error("Failed to record outbox retry",
					"event_id", event.ID,
					"error", markErr,
				)
				return markErr
			}
			continue
		}

		if err := p.outboxRepo.MarkPublished(ctx, tx, event.ID); err != nil {
			slog.Error("Failed to mark outbox event as published",
				"event_id", event.ID,
				"error", err,
			)
			return err
		}

		publishedCount++

		slog.Debug("Published outbox event",
			"event_id", event.ID,
			"event_type", event.EventType,
		)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if publishedCount > 0 {
		slog.Info("Outbox publisher batch completed", "published_count", publishedCount)
	}
	return nil
}
