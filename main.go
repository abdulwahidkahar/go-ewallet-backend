package main

import (
	"fmt"
	"go-ewallet-backend/internal/database"
	"go-ewallet-backend/internal/handler"
	"go-ewallet-backend/internal/repository"
	"go-ewallet-backend/internal/service"
	"log"
	"os"
	"strings"

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
	authService := service.NewAuthService(db, repository.NewUserRepository(db), walletRepo)
	idempotencyService := service.NewIdempotencyService(idempotencyRepo)
	walletService := service.NewWalletService(db, walletRepo, topUpRepo, ledgerRepo, idempotencyService)
	authHandler := handler.NewAuthHandler(authService, rdb)
	walletHandler := handler.NewWalletHandler(walletService)

	r.GET("/health", authHandler.Health)
	r.POST("/register", authHandler.Register)
	r.POST("/login", authHandler.Login)

	api := r.Group("/api")
	api.Use(handler.JWTMiddleware(rdb))
	{
		api.GET("/profile", authHandler.Profile)
		api.POST("/logout", authHandler.Logout)
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
