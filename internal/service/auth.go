package service

import (
	"context"
	"database/sql"
	"errors"
	"go-ewallet-backend/internal/database"
	"go-ewallet-backend/internal/repository"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid email or password")
var ErrUserNotFound = errors.New("user not found")
var ErrTokenGeneration = errors.New("error generating token")

type AuthService struct {
	db         *sql.DB
	userRepo   *repository.UserRepository
	walletRepo *repository.WalletRepository
}

func NewAuthService(db *sql.DB, userRepo *repository.UserRepository, walletRepo *repository.WalletRepository) *AuthService {
	return &AuthService{
		db:         db,
		userRepo:   userRepo,
		walletRepo: walletRepo,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return 0, err
	}

	userID, err := s.userRepo.CreateTx(ctx, tx, email, string(passwordHash))
	if err != nil {
		return 0, err
	}

	_, err = s.walletRepo.CreateTx(ctx, tx, userID)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {

	userID, passwordHash, err := s.userRepo.GetByEmail(ctx, email)

	if err != nil {
		return "", err
	}

	if userID == 0 {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := generateToken(userID, email)
	if err != nil {
		return "", ErrTokenGeneration
	}

	return token, nil
}

func (s *AuthService) Logout(ctx context.Context, rdb *redis.Client, tokenString string) error {

	token := strings.TrimPrefix(tokenString, "Bearer ")

	redis := database.BlacklistToken(ctx, rdb, token, 24*time.Hour)

	return redis
}

func (s *AuthService) Profile(ctx context.Context, userID int) (string, error) {

	email, _, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if email == "" {
		return "", ErrUserNotFound
	}

	return email, nil
}

func generateToken(id int, email string) (string, error) {
	secret := os.Getenv("JWT_SECRET")

	if secret == "" {
		return "", errors.New("JWT_SECRET environment variable not set")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":    id,
		"email": email,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	})

	return token.SignedString([]byte(secret))
}
