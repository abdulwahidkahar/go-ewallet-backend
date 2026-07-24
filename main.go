package main

import (
	"context"
	"fmt"
	"go-ewallet-backend/internal/database"
	"go-ewallet-backend/internal/handler"
	"go-ewallet-backend/internal/repository"
	"go-ewallet-backend/internal/service"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("error loading env")
	}

	r := gin.Default()

	trustedProxies := []string{"127.0.0.1", "::1"}
	if raw := os.Getenv("TRUSTED_PROXIES"); raw != "" {
		trustedProxies = splitAndTrim(raw)
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		log.Fatal("error setting trusted proxies:", err)
	}

	rdb := database.NewRedisClient()

	db, err := database.NewPostgresDB()
	if err != nil {
		log.Fatal("error connecting to database:", err)
	}

	if err == nil {
		fmt.Println("Connected to PostgreSQL database successfully!")
	}
	defer db.Close()

	walletRepo := repository.NewWalletRepository(db)
	topUpRepo := repository.NewTopUpRepository(db)
	ledgerRepo := repository.NewLedgerRepository(db)
	idempotencyRepo := repository.NewIdempotencyRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	authService := service.NewAuthService(db, repository.NewUserRepository(db), walletRepo, refreshTokenRepo)
	idempotencyService := service.NewIdempotencyService(idempotencyRepo)
	walletService := service.NewWalletService(db, walletRepo, topUpRepo, ledgerRepo, idempotencyService, outboxRepo)
	authHandler := handler.NewAuthHandler(authService, rdb)
	walletHandler := handler.NewWalletHandler(walletService)

	// Start outbox publisher worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outboxPublisher := service.NewOutboxPublisher(db, rdb, outboxRepo)
	go outboxPublisher.Start(ctx)

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		fmt.Println("Shutting down outbox publisher...")
		cancel()
	}()

	r.GET("/health", authHandler.Health)
	r.POST("/register", authHandler.Register)
	r.POST("/login", authHandler.Login)
	r.POST("/logout", authHandler.Logout)

	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)
	r.POST("/auth/refresh", authHandler.Refresh)
	r.POST("/auth/logout", authHandler.Logout)

	api := r.Group("/api")
	api.Use(handler.JWTMiddleware(rdb))
	{
		api.GET("/profile", authHandler.Profile)
		api.POST("/logout", authHandler.Logout)
		api.GET("/auth/devices", authHandler.GetDevices)
		api.DELETE("/auth/devices/:id", authHandler.RevokeDevice)
		api.POST("/wallet/topup", walletHandler.CreateTopUp)
		api.GET("/wallet/topup/history", walletHandler.GetTopUpOrders)
		api.POST("/wallet/topups", walletHandler.CreateTopUp)
		api.POST("/wallet/topups/:reference_id/confirm", walletHandler.ConfirmTopUp)
		api.GET("/wallet/topups", walletHandler.GetTopUpOrders)
		api.GET("/wallet/ledger", walletHandler.GetLedgerEntries)
		api.GET("/wallet/balance", walletHandler.GetBalance)
		api.POST("/wallet/transfer", walletHandler.Transfer)
		api.GET("/wallet/transfer", walletHandler.GetHistoryTransfer)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "9090"
	}

	if err := r.Run(":" + port); err != nil {
		log.Fatal("error running server:", err)
	}
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
