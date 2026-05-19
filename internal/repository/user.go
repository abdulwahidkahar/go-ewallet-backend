package repository

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/model"

	"github.com/lib/pq"
)

var ErrEmailAlreadyRegistered = errors.New("email already registered")

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateTx(ctx context.Context, tx *sql.Tx, email, passwordHash string) (int, error) {
	var userID int

	err := tx.QueryRowContext(
		ctx,
		"INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id",
		email,
		passwordHash,
	).Scan(&userID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return 0, ErrEmailAlreadyRegistered
		}

		return 0, err
	}

	return userID, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (int, string, error) {

	user := model.User{}

	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, password FROM users WHERE email = $1",
		email,
	).Scan(&user.ID, &user.Password)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil
		}
		return 0, "", err
	}

	return user.ID, user.Password, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int) (string, string, error) {
	var email, passwordHash string

	err := r.db.QueryRowContext(
		ctx,
		"SELECT email, password FROM users WHERE id = $1",
		id,
	).Scan(&email, &passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil
		}
		return "", "", err
	}

	return email, passwordHash, nil
}
