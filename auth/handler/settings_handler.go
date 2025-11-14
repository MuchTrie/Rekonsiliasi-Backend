package handler

import (
	"net/http"

	"github.com/ciptami/switching-reconcile-web/auth/database"
	"github.com/ciptami/switching-reconcile-web/auth/models"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct{}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
}

// Request DTOs
type UpdateSettingRequest struct {
	IsEnabled bool `json:"is_enabled"`
}

type UpdateSettingsRequest struct {
	Settings map[string]bool `json:"settings"`
}

// Response DTOs
type SettingsResponse struct {
	Settings map[string]bool `json:"settings"`
}

// GetSettings returns all feature settings
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	db := database.GetDB()

	var settings []models.Setting
	if err := db.Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch settings"})
		return
	}

	// Convert to map for easier frontend consumption
	settingsMap := make(map[string]bool)
	for _, setting := range settings {
		settingsMap[setting.FeatureName] = setting.IsEnabled
	}

	c.JSON(http.StatusOK, SettingsResponse{
		Settings: settingsMap,
	})
}

// UpdateSettings updates multiple settings at once (Admin only)
func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	// Check if user is admin (middleware should handle this)
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	claims, ok := userInterface.(*Claims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	// Update each setting
	for featureName, isEnabled := range req.Settings {
		var setting models.Setting
		if err := db.Where("feature_name = ?", featureName).First(&setting).Error; err != nil {
			// If setting doesn't exist, create it
			setting = models.Setting{
				FeatureName: featureName,
				IsEnabled:   isEnabled,
			}
			if err := db.Create(&setting).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create setting"})
				return
			}
		} else {
			// Update existing setting
			setting.IsEnabled = isEnabled
			if err := db.Save(&setting).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update setting"})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// GetSetting returns a specific setting
func (h *SettingsHandler) GetSetting(c *gin.Context) {
	featureName := c.Param("feature")
	
	db := database.GetDB()

	var setting models.Setting
	if err := db.Where("feature_name = ?", featureName).First(&setting).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Setting not found"})
		return
	}

	c.JSON(http.StatusOK, setting)
}

// UpdateSetting updates a specific setting (Admin only)
func (h *SettingsHandler) UpdateSetting(c *gin.Context) {
	// Check if user is admin
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	claims, ok := userInterface.(*Claims)
	if !ok || claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	featureName := c.Param("feature")
	
	var req UpdateSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()

	var setting models.Setting
	if err := db.Where("feature_name = ?", featureName).First(&setting).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Setting not found"})
		return
	}

	setting.IsEnabled = req.IsEnabled
	if err := db.Save(&setting).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update setting"})
		return
	}

	c.JSON(http.StatusOK, setting)
}
