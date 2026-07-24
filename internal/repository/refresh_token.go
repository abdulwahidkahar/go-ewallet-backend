package repository

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"
	"time"
)

var ErrRefreshTokenNotFound = errors.New("refresh token not found")

type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *model.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, device_id, user_agent, ip_address, is_revoked, expires_at, created_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	now := time.Now()
	if token.CreatedAt.IsZero() {
		token.CreatedAt = now
	}
	if token.LastUsedAt.IsZero() {
		token.LastUsedAt = now
	}

	return r.db.QueryRowContext(
		ctx,
		query,
		token.UserID,
		token.TokenHash,
		token.DeviceID,
		token.UserAgent,
		token.IPAddress,
		token.IsRevoked,
		token.ExpiresAt,
		token.CreatedAt,
		token.LastUsedAt,
	).Scan(&token.ID)
}

func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, device_id, user_agent, ip_address, is_revoked, expires_at, created_at, last_used_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var token model.RefreshToken
	var deviceID, userAgent, ipAddress sql.NullString

	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&deviceID,
		&userAgent,
		&ipAddress,
		&token.IsRevoked,
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.LastUsedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, err
	}

	if deviceID.Valid {
		token.DeviceID = deviceID.String
	}
	if userAgent.Valid {
		token.UserAgent = userAgent.String
	}
	if ipAddress.Valid {
		token.IPAddress = ipAddress.String
	}

	return &token, nil
}

func (r *RefreshTokenRepository) RevokeByHash(ctx context.Context, tokenHash string) error {
	query := `
		UPDATE refresh_tokens
		SET is_revoked = TRUE
		WHERE token_hash = $1
	`
	_, err := r.db.ExecContext(ctx, query, tokenHash)
	return err
}

func (r *RefreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID int64) error {
	query := `
		UPDATE refresh_tokens
		SET is_revoked = TRUE
		WHERE user_id = $1 AND is_revoked = FALSE
	`
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *RefreshTokenRepository) GetActiveSessionsByUserID(ctx context.Context, userID int64) ([]model.DeviceSessionResponse, error) {
	query := `
		SELECT id, device_id, user_agent, ip_address, is_revoked, expires_at, created_at, last_used_at
		FROM refresh_tokens
		WHERE user_id = $1 AND is_revoked = FALSE AND expires_at > NOW()
		ORDER BY last_used_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.DeviceSessionResponse
	for rows.Next() {
		var s model.DeviceSessionResponse
		var deviceID, userAgent, ipAddress sql.NullString
		if err := rows.Scan(
			&s.ID,
			&deviceID,
			&userAgent,
			&ipAddress,
			&s.IsRevoked,
			&s.ExpiresAt,
			&s.CreatedAt,
			&s.LastUsedAt,
		); err != nil {
			return nil, err
		}
		if deviceID.Valid {
			s.DeviceID = deviceID.String
		}
		if userAgent.Valid {
			s.UserAgent = userAgent.String
		}
		if ipAddress.Valid {
			s.IPAddress = ipAddress.String
		}
		sessions = append(sessions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (r *RefreshTokenRepository) RevokeSessionByID(ctx context.Context, userID int64, sessionID int64) error {
	query := `
		UPDATE refresh_tokens
		SET is_revoked = TRUE
		WHERE id = $1 AND user_id = $2
	`
	result, err := r.db.ExecContext(ctx, query, sessionID, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRefreshTokenNotFound
	}

	return nil
}
