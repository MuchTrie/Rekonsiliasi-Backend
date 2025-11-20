package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ciptami/switching-reconcile-web/internal/handler"
	"github.com/ciptami/switching-reconcile-web/internal/middleware"
	"github.com/ciptami/switching-reconcile-web/internal/service"
	"github.com/ciptami/switching-reconcile-web/pkg/validator"

	// Auth imports
	"github.com/ciptami/switching-reconcile-web/auth/database"
	"github.com/ciptami/switching-reconcile-web/auth/seeder"
	authHandler "github.com/ciptami/switching-reconcile-web/auth/handler"
	authMiddleware "github.com/ciptami/switching-reconcile-web/auth/middleware"
	
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		fmt.Println("Warning: .env file not found, using system environment variables")
	}
	
	// Initialize logger
	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetLevel(logrus.InfoLevel)
	
	log.Info("Starting Switching Reconciliation Web Server...")
	
	// Connect to database
	if err := database.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	
	// Run database migrations
	if err := database.AutoMigrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	
	// Run seeders
	if err := seeder.RunAll(); err != nil {
		log.Fatalf("Failed to run seeders: %v", err)
	}
	
	// Get executable directory to ensure paths are relative to backend folder
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	baseDir := filepath.Dir(exePath)
	
	// If running from bin folder, go up one level to backend folder
	if filepath.Base(baseDir) == "bin" {
		baseDir = filepath.Dir(baseDir)
	}
	
	fmt.Println("\n" + strings.Repeat("=", 60))
	log.Infof("Base directory: %s", baseDir)
	
	// Create required directories in backend folder
	uploadDir := filepath.Join(baseDir, "uploads")
	resultsDir := filepath.Join(baseDir, "results")
	
	dirs := []string{uploadDir, resultsDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			log.Fatalf("❌ Failed to create directory %s: %v", dir, err)
		}
		log.Printf("✅ Directory ready: %s\n", dir)
	}

	// ==========================================
	// 🧹 CLEANUP SERVICE INITIALIZATION
	// ==========================================
	printCleanupHeader()

	// Get retention days from environment (default: 3)
	retentionDays := 3
	if envDays := os.Getenv("RETENTION_DAYS"); envDays != "" {
		if days, err := strconv.Atoi(envDays); err == nil && days > 0 {
			retentionDays = days
		}
	}

	cleanupService := service.NewCleanupService(resultsDir)

	// Check if auto-cleanup is enabled (default: true)
	autoCleanupEnabled := os.Getenv("AUTO_CLEANUP") != "false"

	if autoCleanupEnabled {
		// ==========================================
		// OPSI 1: AUTO-CLEANUP ON STARTUP
		// ==========================================
		log.Printf("🔍 Mengecek hasil rekonsiliasi yang lebih dari %d hari...\n", retentionDays)

		oldFolders, err := cleanupService.GetOldFolders(retentionDays)
		if err != nil {
			log.Printf("⚠️  Error saat mengecek folder lama: %v\n", err)
		} else if len(oldFolders) == 0 {
			log.Printf("✅ Tidak ada folder yang lebih dari %d hari\n", retentionDays)
		} else {
			// Tampilkan preview
			var totalSize int64
			log.Printf("\n📁 Ditemukan %d folder yang akan dihapus:\n", len(oldFolders))
			for i, folder := range oldFolders {
				log.Printf("  [%d] %s (%d hari, %d files, %.2f MB)\n",
					i+1, folder.Name, folder.DaysOld, folder.FileCount, folder.SizeMB)
				totalSize += folder.SizeBytes
			}
			log.Printf("💾 Total ukuran: %.2f MB\n\n", float64(totalSize)/(1024*1024))

			// Langsung hapus tanpa konfirmasi
			log.Println("🗑️  Memulai proses cleanup...")
			deletedCount, err := cleanupService.AutoCleanup(retentionDays)
			if err != nil {
				log.Printf("⚠️  Error saat cleanup: %v\n", err)
			} else if deletedCount > 0 {
				log.Printf("✅ Cleanup startup selesai\n")
			}
		}

		// ==========================================
		// OPSI 2: SCHEDULED CLEANUP (DAILY)
		// ==========================================
		go startScheduledCleanup(cleanupService, retentionDays)
	} else {
		log.Println("⏭️  Auto-cleanup disabled (set AUTO_CLEANUP=true to enable)")
	}

	printSeparator()

	// ==========================================
	// 🌐 INITIALIZE HANDLERS & ROUTER
	// ==========================================
	printServerHeader()

	// Initialize logger for services
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	// Initialize dependencies with absolute paths
	fileValidator := validator.NewFileValidator()
	reconService := service.NewReconciliationService(logger, uploadDir, resultsDir)
	reconHandler := handler.NewReconciliationHandler(reconService, fileValidator, logger)

	// Initialize auth dependencies
	authHandlerInstance := authHandler.NewAuthHandler()
	settingsHandlerInstance := authHandler.NewSettingsHandler()
	
	// Setup Gin router
	router := gin.Default()
	
	// Apply CORS middleware
	router.Use(middleware.CORSMiddleware())
	
	// API routes
	api := router.Group("/api")
	{
		// Health check
		api.GET("/health", reconHandler.HealthCheck)
		
		// Auth endpoints (public)
		api.POST("/auth/login", authHandlerInstance.Login)
		api.POST("/auth/register", authHandlerInstance.Register)
		
		// Protected routes - require authentication
		protected := api.Group("")
		protected.Use(authMiddleware.AuthMiddleware())
		{
			// Current user info
			protected.GET("/auth/me", authHandlerInstance.GetMe)
			
			// Profile management
			protected.PUT("/profile", authHandlerInstance.UpdateProfile)
			protected.PUT("/change-password", authHandlerInstance.ChangePassword)
			
			// Settings endpoints
			protected.GET("/settings", settingsHandlerInstance.GetSettings)
			protected.GET("/settings/:feature", settingsHandlerInstance.GetSetting)
			
			// Admin only - update settings
			adminProtected := protected.Group("")
			adminProtected.Use(authMiddleware.RoleMiddleware("admin"))
			{
				adminProtected.PUT("/settings", settingsHandlerInstance.UpdateSettings)
				adminProtected.PUT("/settings/:feature", settingsHandlerInstance.UpdateSetting)
			}
			
			// Reconciliation endpoints - Admin dan Operasional
			protected.POST("/reconcile", reconHandler.ProcessReconciliation)
			protected.GET("/job/:jobId", reconHandler.GetJobStatus)
			protected.GET("/results", reconHandler.GetResultFolders)
			protected.GET("/results/:jobId/:vendor/:type", reconHandler.GetResultData)
			protected.GET("/download/:jobId/:filename", reconHandler.DownloadResult)
			
			// Duplicate detection endpoints
			protected.GET("/duplicates/:job_id", reconHandler.GetDuplicateReport)
			protected.GET("/duplicates/:job_id/download", reconHandler.DownloadDuplicateReport)
			
			// Settlement conversion endpoint - Admin dan Operasional
			protected.POST("/convert/settlement", reconHandler.ConvertSettlement)
			protected.GET("/converted/files", reconHandler.GetConvertedFiles)
			protected.GET("/preview/converted/:filename", reconHandler.PreviewConvertedFile)
		}
	}
	
	// Download converted files (protected)
	router.GET("/api/download/converted/:filename", authMiddleware.AuthMiddleware(), func(c *gin.Context) {
		filename := c.Param("filename")
		filePath := filepath.Join(resultsDir, "converted", filename)
		c.File(filePath)
	})
	
	// Static files (for serving frontend in production)
	router.Static("/static", "./frontend/dist")
	router.StaticFile("/", "./frontend/dist/index.html")

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("🚀 Server is running on http://localhost%s\n", addr)
	log.Printf("📖 API Documentation: http://localhost%s/api/health\n", addr)
	printSeparator()

	if err := router.Run(addr); err != nil {
		log.Fatalf("❌ Failed to start server: %v", err)
	}
}

// ==========================================
// SCHEDULED CLEANUP GOROUTINE
// ==========================================
func startScheduledCleanup(cleanupService *service.CleanupService, retentionDays int) {
	log.Println("⏰ Scheduled cleanup dimulai (setiap hari jam 00:00)")

	// Hitung waktu untuk cleanup pertama (jam 00:00 besok)
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	durationUntilMidnight := time.Until(nextMidnight)

	// Tunggu sampai tengah malam pertama
	time.Sleep(durationUntilMidnight)

	// Cleanup pertama
	runScheduledCleanup(cleanupService, retentionDays)

	// Lalu jalankan setiap 24 jam
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runScheduledCleanup(cleanupService, retentionDays)
	}
}

func runScheduledCleanup(cleanupService *service.CleanupService, retentionDays int) {
	log.Println("\n========================================")
	log.Println("🕛 SCHEDULED CLEANUP - " + time.Now().Format("2006-01-02 15:04:05"))
	log.Println("========================================")

	deletedCount, err := cleanupService.AutoCleanup(retentionDays)
	if err != nil {
		log.Printf("⚠️  Scheduled cleanup error: %v\n", err)
	} else if deletedCount > 0 {
		log.Printf("✅ Scheduled cleanup selesai: %d folder dihapus\n", deletedCount)
	} else {
		log.Printf("✅ Tidak ada folder yang perlu dihapus\n")
	}

	log.Println("========================================\n")
}

// ==========================================
// HELPER FUNCTIONS
// ==========================================

func printBanner() {
	fmt.Println("\n========================================")
	fmt.Println("   🚀 SWITCHING RECONCILIATION SERVER")
	fmt.Println("========================================\n")
}

func printSeparator() {
	fmt.Println("========================================\n")
}

func printCleanupHeader() {
	fmt.Println("\n========================================")
	fmt.Println("        🧹 CLEANUP SERVICE")
	fmt.Println("========================================")
}

func printServerHeader() {
	fmt.Println("========================================")
	fmt.Println("      🌐 STARTING WEB SERVER")
	fmt.Println("========================================\n")
	time.Sleep(300 * time.Millisecond)
}
