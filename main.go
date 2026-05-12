package main

import (
	"auth-api/internal/database"
	"auth-api/internal/handler"
	"fmt"
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

	authHandler := handler.NewAuthHandler(db, rdb)
	walletHandler := handler.NewWalletHandler(db)

	r.GET("/health", authHandler.Health)
	r.POST("/register", authHandler.Register)
	r.POST("/login", authHandler.Login)

	api := r.Group("/api")
	api.Use(handler.JWTMiddleware(rdb))
	{
		api.GET("/profile", authHandler.Profile)
		api.POST("/logout", authHandler.Logout)
		api.POST("/wallet/topup", walletHandler.TopUp)
		api.GET("/wallet/balance", walletHandler.GetBalance)
		api.POST("/wallet/transfer", walletHandler.Transfer)
		api.GET("/wallet/transfer", walletHandler.GetHistoryTransfer)
	}

	r.Run(":9090")
}
