package main

import (
	"fmt"
	"os"
	"path/filepath"

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
	
	// Initialize dependencies with absolute paths
	fileValidator := validator.NewFileValidator()
	reconService := service.NewReconciliationService(log, uploadDir, resultsDir)
	reconHandler := handler.NewReconciliationHandler(reconService, fileValidator, log)
	
	// Initialize auth handlers
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
	log.Infof("Server is running on http://localhost%s", addr)
	log.Infof("API Documentation: http://localhost%s/api/health", addr)
	
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
