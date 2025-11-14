package seeder

import (
	"log"

	"github.com/ciptami/switching-reconcile-web/auth/database"
	"github.com/ciptami/switching-reconcile-web/auth/models"
	"golang.org/x/crypto/bcrypt"
)

// SeedUsers creates default users if they don't exist
func SeedUsers() error {
	db := database.GetDB()

	// Check if users already exist
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		log.Println("ℹ️  Users already exist, skipping seed")
		return nil
	}

	log.Println("🌱 Seeding users...")

	// Default users
	users := []struct {
		Email    string
		Password string
		Role     string
	}{
		{"admin@switching.com", "admin123", "admin"},
		{"operasional@switching.com", "operasional123", "operasional"},
	}

	for _, userData := range users {
		// Hash password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(userData.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		user := models.User{
			Email:    userData.Email,
			Password: string(hashedPassword),
			Role:     userData.Role,
		}

		if err := db.Create(&user).Error; err != nil {
			return err
		}

		log.Printf("✅ Created user: %s (password: %s)", userData.Email, userData.Password)
	}

	return nil
}

// SeedSettings creates default settings if they don't exist
func SeedSettings() error {
	db := database.GetDB()

	// Check if settings already exist
	var count int64
	db.Model(&models.Setting{}).Count(&count)
	if count > 0 {
		log.Println("ℹ️  Settings already exist, skipping seed")
		return nil
	}

	log.Println("🌱 Seeding settings...")

	// Default settings
	settings := []models.Setting{
		{
			FeatureName: "reconciliation",
			IsEnabled:   true,
			Description: "Enable/disable reconciliation feature",
		},
		{
			FeatureName: "settlement_converter",
			IsEnabled:   true,
			Description: "Enable/disable settlement converter feature",
		},
		{
			FeatureName: "result_history",
			IsEnabled:   true,
			Description: "Enable/disable result history feature",
		},
	}

	for _, setting := range settings {
		if err := db.Create(&setting).Error; err != nil {
			return err
		}
		log.Printf("✅ Created setting: %s (enabled: %v)", setting.FeatureName, setting.IsEnabled)
	}

	return nil
}

// RunAll runs all seeders
func RunAll() error {
	log.Println("🌱 Starting database seeding...")

	if err := SeedUsers(); err != nil {
		return err
	}

	if err := SeedSettings(); err != nil {
		return err
	}

	log.Println("✅ Database seeding completed successfully")
	return nil
}
