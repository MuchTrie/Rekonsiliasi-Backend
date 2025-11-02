package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ciptami/switching-reconcile-web/internal/handler"
	"github.com/ciptami/switching-reconcile-web/internal/middleware"
	"github.com/ciptami/switching-reconcile-web/internal/service"
	"github.com/ciptami/switching-reconcile-web/pkg/validator"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger
	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetLevel(logrus.InfoLevel)
	
	log.Info("Starting Switching Reconciliation Web Server...")
	
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
	
	// Setup Gin router
	router := gin.Default()
	
	// Apply CORS middleware
	router.Use(middleware.CORSMiddleware())
	
	// API routes
	api := router.Group("/api")
	{
		// Health check
		api.GET("/health", reconHandler.HealthCheck)
		
		// Reconciliation endpoints
		api.POST("/reconcile", reconHandler.ProcessReconciliation)
		api.GET("/job/:jobId", reconHandler.GetJobStatus)
		api.GET("/results", reconHandler.GetResultFolders)
		api.GET("/results/:jobId/:vendor/:type", reconHandler.GetResultData)
		api.GET("/download/:jobId/:filename", reconHandler.DownloadResult)
		
		// Settlement conversion endpoint
		api.POST("/convert/settlement", reconHandler.ConvertSettlement)
		api.GET("/converted/files", reconHandler.GetConvertedFiles)
	}
	
	// Download converted files
	router.GET("/api/download/converted/:filename", func(c *gin.Context) {
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
