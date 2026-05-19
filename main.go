package main

import (
	"fmt"
	"go-ewallet-backend/internal/database"
	"go-ewallet-backend/internal/handler"
	"go-ewallet-backend/internal/repository"
	"go-ewallet-backend/internal/service"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {

	r := gin.Default()

	if err := godotenv.Load(); err != nil {
		fmt.Println("error loading env")
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
		api.GET("/wallet/balance", walletHandler.GetBalance)
		api.POST("/wallet/transfer", walletHandler.Transfer)
		api.GET("/wallet/transfer", walletHandler.GetHistoryTransfer)
	}

	r.Run(":9090")
}
