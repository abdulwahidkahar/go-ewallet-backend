package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"go-ewallet-backend/internal/database"
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrUserNotFound        = errors.New("user not found")
	ErrTokenGeneration     = errors.New("error generating token")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenRevoked = errors.New("refresh token has been revoked")
	ErrRefreshTokenExpired = errors.New("refresh token has expired")
)

const (
	AccessTokenDuration  = 15 * time.Minute
	RefreshTokenDuration = 30 * 24 * time.Hour
)

type AuthService struct {
	db               *sql.DB
	userRepo         *repository.UserRepository
	walletRepo       *repository.WalletRepository
	refreshTokenRepo *repository.RefreshTokenRepository
}

func NewAuthService(
	db *sql.DB,
	userRepo *repository.UserRepository,
	walletRepo *repository.WalletRepository,
	refreshTokenRepo *repository.RefreshTokenRepository,
) *AuthService {
	return &AuthService{
		db:               db,
		userRepo:         userRepo,
		walletRepo:       walletRepo,
		refreshTokenRepo: refreshTokenRepo,
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

func (s *AuthService) Login(ctx context.Context, email, password, deviceID, userAgent, ipAddress string) (*model.AuthResponse, error) {
	userID, passwordHash, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if userID == 0 {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, err := generateToken(userID, email, AccessTokenDuration)
	if err != nil {
		return nil, ErrTokenGeneration
	}

	rawRefreshToken, err := generateRandomToken()
	if err != nil {
		return nil, ErrTokenGeneration
	}

	tokenHash := HashToken(rawRefreshToken)

	if s.refreshTokenRepo != nil {
		refreshTokenModel := &model.RefreshToken{
			UserID:     int64(userID),
			TokenHash:  tokenHash,
			DeviceID:   deviceID,
			UserAgent:  userAgent,
			IPAddress:  ipAddress,
			IsRevoked:  false,
			ExpiresAt:  time.Now().Add(RefreshTokenDuration),
			CreatedAt:  time.Now(),
			LastUsedAt: time.Now(),
		}

		if err := s.refreshTokenRepo.Create(ctx, refreshTokenModel); err != nil {
			return nil, fmt.Errorf("failed to save refresh token: %w", err)
		}
	}

	return &model.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(AccessTokenDuration.Seconds()),
	}, nil
}

func (s *AuthService) Refresh(ctx context.Context, rawRefreshToken, deviceID, userAgent, ipAddress string) (*model.AuthResponse, error) {
	if s.refreshTokenRepo == nil {
		return nil, errors.New("refresh token repository not configured")
	}

	tokenHash := HashToken(rawRefreshToken)
	storedToken, err := s.refreshTokenRepo.GetByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	if storedToken.IsRevoked {
		// Potential token reuse attack - revoke all user's sessions for security
		_ = s.refreshTokenRepo.RevokeAllByUserID(ctx, storedToken.UserID)
		return nil, ErrRefreshTokenRevoked
	}

	if time.Now().After(storedToken.ExpiresAt) {
		return nil, ErrRefreshTokenExpired
	}

	// Revoke old refresh token (Refresh Token Rotation)
	if err := s.refreshTokenRepo.RevokeByHash(ctx, tokenHash); err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

	email, _, err := s.userRepo.GetByID(ctx, int(storedToken.UserID))
	if err != nil || email == "" {
		return nil, ErrUserNotFound
	}

	newAccessToken, err := generateToken(int(storedToken.UserID), email, AccessTokenDuration)
	if err != nil {
		return nil, ErrTokenGeneration
	}

	newRawRefreshToken, err := generateRandomToken()
	if err != nil {
		return nil, ErrTokenGeneration
	}

	newTokenHash := HashToken(newRawRefreshToken)

	effectiveDeviceID := deviceID
	if effectiveDeviceID == "" {
		effectiveDeviceID = storedToken.DeviceID
	}
	effectiveUserAgent := userAgent
	if effectiveUserAgent == "" {
		effectiveUserAgent = storedToken.UserAgent
	}
	effectiveIPAddress := ipAddress
	if effectiveIPAddress == "" {
		effectiveIPAddress = storedToken.IPAddress
	}

	newTokenModel := &model.RefreshToken{
		UserID:     storedToken.UserID,
		TokenHash:  newTokenHash,
		DeviceID:   effectiveDeviceID,
		UserAgent:  effectiveUserAgent,
		IPAddress:  effectiveIPAddress,
		IsRevoked:  false,
		ExpiresAt:  time.Now().Add(RefreshTokenDuration),
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}

	if err := s.refreshTokenRepo.Create(ctx, newTokenModel); err != nil {
		return nil, fmt.Errorf("failed to save new refresh token: %w", err)
	}

	return &model.AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRawRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(AccessTokenDuration.Seconds()),
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, rdb *redis.Client, authHeader string, rawRefreshToken string) error {
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		_ = database.BlacklistToken(ctx, rdb, token, AccessTokenDuration)
	}

	if rawRefreshToken != "" && s.refreshTokenRepo != nil {
		tokenHash := HashToken(rawRefreshToken)
		_ = s.refreshTokenRepo.RevokeByHash(ctx, tokenHash)
	}

	return nil
}

func (s *AuthService) GetActiveDevices(ctx context.Context, userID int64) ([]model.DeviceSessionResponse, error) {
	if s.refreshTokenRepo == nil {
		return nil, errors.New("refresh token repository not configured")
	}
	return s.refreshTokenRepo.GetActiveSessionsByUserID(ctx, userID)
}

func (s *AuthService) RevokeDevice(ctx context.Context, userID int64, sessionID int64) error {
	if s.refreshTokenRepo == nil {
		return errors.New("refresh token repository not configured")
	}
	return s.refreshTokenRepo.RevokeSessionByID(ctx, userID, sessionID)
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

func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func generateRandomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func generateToken(id int, email string, duration time.Duration) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", errors.New("JWT_SECRET environment variable not set")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":    id,
		"email": email,
		"exp":   time.Now().Add(duration).Unix(),
	})

	return token.SignedString([]byte(secret))
}
