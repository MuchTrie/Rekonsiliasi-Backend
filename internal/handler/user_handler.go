package handler

import (
	"net/http"

	"github.com/ciptami/switching-reconcile-web/auth/database"
	"github.com/ciptami/switching-reconcile-web/auth/models"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type UserHandler struct {
	log *logrus.Logger
}

func NewUserHandler(log *logrus.Logger) *UserHandler {
	return &UserHandler{
		log: log,
	}
}

type UserCountResponse struct {
	TotalUsers       int `json:"totalUsers"`
	AdminCount       int `json:"adminCount"`
	OperationalCount int `json:"operationalCount"`
}

// GetUserCount returns user statistics
func (h *UserHandler) GetUserCount(c *gin.Context) {
	db := database.GetDB()
	
	var totalUsers int64
	var adminCount int64
	var opsCount int64
	
	// Count total users
	if err := db.Model(&models.User{}).Count(&totalUsers).Error; err != nil {
		h.log.Errorf("❌ Failed to count total users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get user count",
		})
		return
	}
	
	// Count admin users
	if err := db.Model(&models.User{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
		h.log.Errorf("❌ Failed to count admin users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get admin count",
		})
		return
	}
	
	// Count operational users
	if err := db.Model(&models.User{}).Where("role = ?", "operasional").Count(&opsCount).Error; err != nil {
		h.log.Errorf("❌ Failed to count operational users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to get operational count",
		})
		return
	}
	
	h.log.Infof("👥 User Count - Total: %d, Admin: %d, Operasional: %d", totalUsers, adminCount, opsCount)
	
	response := UserCountResponse{
		TotalUsers:       int(totalUsers),
		AdminCount:       int(adminCount),
		OperationalCount: int(opsCount),
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}
