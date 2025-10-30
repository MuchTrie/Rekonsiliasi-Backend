package main

import (
	"fmt"
	"os"

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
	
	// Create required directories
	dirs := []string{"uploads", "results"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}
	
	// Initialize dependencies
	fileValidator := validator.NewFileValidator()
	reconService := service.NewReconciliationService(log)
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
	}
	
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
