package handler

import (
	"errors"
	"go-ewallet-backend/internal/model"
	"go-ewallet-backend/internal/repository"
	"go-ewallet-backend/internal/service"
	"net/http"
	"strconv"

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

type LoginRequestBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	DeviceID string `json:"device_id,omitempty"`
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
		statusCode, message := mapAuthError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully", "email": req.Email})
}

func (ah *AuthHandler) Login(c *gin.Context) {
	var req LoginRequestBody

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

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = c.GetHeader("X-Device-ID")
	}

	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	authResp, err := ah.userService.Login(c.Request.Context(), req.Email, req.Password, deviceID, userAgent, ipAddress)
	if err != nil {
		statusCode, message := mapAuthError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Login successful",
		"token":         authResp.AccessToken,
		"access_token":  authResp.AccessToken,
		"refresh_token": authResp.RefreshToken,
		"token_type":    authResp.TokenType,
		"expires_in":    authResp.ExpiresIn,
	})
}

func (ah *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body, refresh_token is required"})
		return
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = c.GetHeader("X-Device-ID")
	}

	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	authResp, err := ah.userService.Refresh(c.Request.Context(), req.RefreshToken, deviceID, userAgent, ipAddress)
	if err != nil {
		statusCode, message := mapAuthError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Token refreshed successfully",
		"access_token":  authResp.AccessToken,
		"refresh_token": authResp.RefreshToken,
		"token_type":    authResp.TokenType,
		"expires_in":    authResp.ExpiresIn,
	})
}

func (ah *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")

	var req model.LogoutRequest
	_ = c.ShouldBindJSON(&req)

	err := ah.userService.Logout(c.Request.Context(), ah.rdb, authHeader, req.RefreshToken)
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
		statusCode, message := mapAuthError(err)
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email": email,
	})
}

func (ah *AuthHandler) GetDevices(c *gin.Context) {
	userID := int64(c.MustGet("id").(float64))

	sessions, err := ah.userService.GetActiveDevices(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": sessions,
	})
}

func (ah *AuthHandler) RevokeDevice(c *gin.Context) {
	userID := int64(c.MustGet("id").(float64))
	sessionIDParam := c.Param("id")

	sessionID, err := strconv.ParseInt(sessionIDParam, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	err = ah.userService.RevokeDevice(c.Request.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Device session revoked successfully",
	})
}

func (ah *AuthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Server is running"})
}

func mapAuthError(err error) (int, string) {
	switch {
	case errors.Is(err, repository.ErrEmailAlreadyRegistered):
		return http.StatusConflict, err.Error()
	case errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, service.ErrUserNotFound):
		return http.StatusNotFound, "User not found"
	case errors.Is(err, service.ErrTokenGeneration):
		return http.StatusInternalServerError, err.Error()
	case errors.Is(err, service.ErrInvalidRefreshToken):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, service.ErrRefreshTokenRevoked):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, service.ErrRefreshTokenExpired):
		return http.StatusUnauthorized, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
