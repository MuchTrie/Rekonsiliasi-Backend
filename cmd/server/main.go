package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		log.Infof("Directory ready: %s", dir)
	}
	
	fmt.Println(strings.Repeat("=", 60))
	
	// Initialize dependencies with absolute paths
	fileValidator := validator.NewFileValidator()
	reconService := service.NewReconciliationService(log, uploadDir, resultsDir)
	reconHandler := handler.NewReconciliationHandler(reconService, fileValidator, log)
	
	// Initialize cleanup service
	cleanupService := service.NewCleanupService(resultsDir, log)
	
	// Check for old folders and prompt for cleanup
	fmt.Println("\n🧹 PENGECEKAN DATA LAMA")
	fmt.Println(strings.Repeat("-", 60))
	checkAndCleanupOldFolders(cleanupService, log)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()
	
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
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🚀 STARTING SERVER")
	fmt.Println(strings.Repeat("=", 60))
	addr := fmt.Sprintf(":%s", port)
	log.Infof("Server is running on http://localhost%s", addr)
	log.Infof("API Documentation: http://localhost%s/api/health", addr)
	fmt.Println(strings.Repeat("=", 60) + "\n")
	
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func checkAndCleanupOldFolders(cleanupService *service.CleanupService, log *logrus.Logger) {
	retentionDays := 3
	log.Info("🔍 Mengecek hasil rekonsiliasi lama...")
	
	oldFolders, err := cleanupService.CheckOldFolders(retentionDays)
	if err != nil {
		log.Warnf("Gagal memeriksa folder lama: %v", err)
		return
	}
	
	if len(oldFolders) == 0 {
		log.Info("✅ Tidak ada folder yang lebih dari 3 hari")
		return
	}
	
	// Display old folders
	log.Infof("📁 Ditemukan %d folder lebih dari %d hari:", len(oldFolders), retentionDays)
	for i, folder := range oldFolders {
		log.Infof("  [%d] %s (%d hari) - %d file", i+1, folder.Name, folder.Age, folder.FileCount)
	}
	
	// Prompt user for confirmation
	fmt.Print("❓ Apakah ingin menghapus folder ini? (Y/N): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Warnf("Gagal membaca input: %v", err)
		return
	}
	
	// Trim whitespace and check response
	input = strings.TrimSpace(input)
	if strings.ToUpper(input) == "Y" {
		if err := cleanupService.DeleteFolders(oldFolders); err != nil {
			log.Errorf("Gagal menghapus folder: %v", err)
		} else {
			log.Infof("✅ Berhasil menghapus %d folder", len(oldFolders))
		}
	} else {
		log.Info("⏭️  Melewati penghapusan folder")
	}
}
