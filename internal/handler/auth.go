package handler

import (
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type AuthHandler struct {
	userService *service.AuthService
	rdb         *redis.Client
}

func NewAuthHandler(userService *service.AuthService, rdb *redis.Client) *AuthHandler {
	return &AuthHandler{userService: userService, rdb: rdb}
}

func (ah *AuthHandler) Register(c *gin.Context) {

	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and Password are required"})
		return
	}

	_, err := ah.userService.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully", "email": req.Email})
}

func (ah *AuthHandler) Login(c *gin.Context) {

	var req model.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	if req.Email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Email and password are required",
		})
		return
	}

	token, err := ah.userService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   token,
	})
}

func (ah *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")

	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	err := ah.userService.Logout(c.Request.Context(), ah.rdb, authHeader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

func (ah *AuthHandler) Profile(c *gin.Context) {

	userID := int(c.MustGet("id").(float64))

	email, err := ah.userService.Profile(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email": email,
	})
}

func (ah *AuthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Server is running"})
}
