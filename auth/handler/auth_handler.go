package handler

import (
	"net/http"
	"os"
	"time"

	"github.com/ciptami/switching-reconcile-web/auth/database"
	"github.com/ciptami/switching-reconcile-web/auth/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// Request DTOs
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role" binding:"required,oneof=admin operasional"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type UpdateProfileRequest struct {
	Username string `json:"username" binding:"required,min=3"`
	Email    string `json:"email" binding:"required,email"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required,min=6"`
}

// Response DTOs
type AuthResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

type UserResponse struct {
	ID       uint   `json:"id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Username string `json:"username,omitempty"`
}

// JWT Claims
type Claims struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	db := database.GetDB()

	// Check if user already exists
	var existingUser models.User
	if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Email already registered"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to hash password"})
		return
	}

	// Create user
	user := models.User{
		Email:    req.Email,
		Password: string(hashedPassword),
		Role:     req.Role,
	}

	if err := db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create user"})
		return
	}

	// Generate JWT token
	token, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "User registered successfully",
		"data": AuthResponse{
			Token: token,
			User: UserResponse{
				ID:       user.ID,
				Email:    user.Email,
				Role:     user.Role,
				Username: user.Username,
			},
		},
	})
}

// Login handles user login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	db := database.GetDB()

	// Find user by email
	var user models.User
	if err := db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid email or password"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid email or password"})
		return
	}

	// Generate JWT token
	token, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Login successful",
		"data": AuthResponse{
			Token: token,
			User: UserResponse{
				ID:       user.ID,
				Email:    user.Email,
				Role:     user.Role,
				Username: user.Username,
			},
		},
	})
}

// generateToken creates a JWT token for the user
func (h *AuthHandler) generateToken(user models.User) (string, error) {
	// Create claims
	claims := Claims{
		ID:    user.ID,
		Email: user.Email,
		Role:  user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // Token expires in 24 hours
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with secret key
	jwtSecret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GetMe returns current user info
func (h *AuthHandler) GetMe(c *gin.Context) {
	// Get user from context (set by auth middleware)
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	claims, ok := userInterface.(*Claims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user data"})
		return
	}

	c.JSON(http.StatusOK, UserResponse{
		ID:    claims.ID,
		Email: claims.Email,
		Role:  claims.Role,
	})
}

// UpdateProfile updates user profile (username and email)
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	// Get user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Unauthorized"})
		return
	}

	claims, ok := userInterface.(*Claims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid user data"})
		return
	}

	db := database.GetDB()

	// Check if email is being changed and already exists
	if req.Email != claims.Email {
		var existingUser models.User
		if err := db.Where("email = ? AND id != ?", req.Email, claims.ID).First(&existingUser).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Email already in use"})
			return
		}
	}

	// Update user
	updates := map[string]interface{}{
		"username": req.Username,
		"email":    req.Email,
	}

	if err := db.Model(&models.User{}).Where("id = ?", claims.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update profile"})
		return
	}

	// Get updated user
	var user models.User
	if err := db.First(&user, claims.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve updated user"})
		return
	}

	// Generate new token if email changed
	token := ""
	if req.Email != claims.Email {
		var err error
		token, err = h.generateToken(user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to generate new token"})
			return
		}
	}

	response := gin.H{
		"success": true,
		"message": "Profile updated successfully",
		"data": UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			Role:     user.Role,
			Username: user.Username,
		},
	}

	if token != "" {
		response["token"] = token
	}

	c.JSON(http.StatusOK, response)
}

// ChangePassword changes user password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	// Get user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Unauthorized"})
		return
	}

	claims, ok := userInterface.(*Claims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Invalid user data"})
		return
	}

	db := database.GetDB()

	// Get current user
	var user models.User
	if err := db.First(&user, claims.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "User not found"})
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Current password is incorrect"})
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to hash new password"})
		return
	}

	// Update password
	if err := db.Model(&user).Update("password", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Password changed successfully",
	})
}
