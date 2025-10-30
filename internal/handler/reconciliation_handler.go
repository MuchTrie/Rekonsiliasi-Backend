package handler

import (
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/ciptami/switching-reconcile-web/internal/service"
	"github.com/ciptami/switching-reconcile-web/pkg/validator"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ReconciliationHandler handles HTTP requests for reconciliation
type ReconciliationHandler struct {
	service   *service.ReconciliationService
	validator *validator.FileValidator
	log       *logrus.Logger
}

// NewReconciliationHandler creates a new handler
func NewReconciliationHandler(service *service.ReconciliationService, validator *validator.FileValidator, log *logrus.Logger) *ReconciliationHandler {
	return &ReconciliationHandler{
		service:   service,
		validator: validator,
		log:       log,
	}
}

// HealthCheck handles health check request
func (h *ReconciliationHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Switching Reconciliation API is running",
		Data: map[string]string{
			"status":  "healthy",
			"version": "1.0.0",
		},
	})
}

// ProcessReconciliation handles file upload and reconciliation processing
func (h *ReconciliationHandler) ProcessReconciliation(c *gin.Context) {
	h.log.Info("Received reconciliation request")
	
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(200 << 20); err != nil { // 200 MB max
		h.log.Errorf("Failed to parse multipart form: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Failed to parse request",
			Error:   err.Error(),
		})
		return
	}
	
	// Get core file
	coreFile, err := c.FormFile("core_file")
	if err != nil {
		h.log.Errorf("Core file is required: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Core file is required",
			Error:   err.Error(),
		})
		return
	}
	
	// Validate core file
	if err := h.validator.ValidateCSVFile(coreFile); err != nil {
		h.log.Errorf("Core file validation failed: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Core file validation failed",
			Error:   err.Error(),
		})
		return
	}
	
	// Build request
	req := &dto.ReconciliationRequest{
		CoreFile: coreFile,
	}
	
	// Get vendor files (optional)
	vendorFiles := []struct {
		reconKey      string
		settlementKey string
		reconDest     **multipart.FileHeader
		settleDest    **multipart.FileHeader
	}{
		{"alto_recon_file", "alto_settlement_file", &req.AltoReconFile, &req.AltoSettlementFile},
		{"jalin_recon_file", "jalin_settlement_file", &req.JalinReconFile, &req.JalinSettlementFile},
		{"aj_recon_file", "aj_settlement_file", &req.AJReconFile, &req.AJSettlementFile},
		{"rinti_recon_file", "rinti_settlement_file", &req.RintiReconFile, &req.RintiSettlementFile},
	}
	
	for _, vf := range vendorFiles {
		// Recon file
		if reconFile, err := c.FormFile(vf.reconKey); err == nil {
			if err := h.validator.ValidateCSVFile(reconFile); err != nil {
				h.log.Warnf("Skipping %s: %v", vf.reconKey, err)
			} else {
				*vf.reconDest = reconFile
				h.log.Infof("Received %s: %s", vf.reconKey, reconFile.Filename)
			}
		}
		
		// Settlement file
		if settleFile, err := c.FormFile(vf.settlementKey); err == nil {
			if err := h.validator.ValidateFile(settleFile); err != nil {
				h.log.Warnf("Skipping %s: %v", vf.settlementKey, err)
			} else {
				*vf.settleDest = settleFile
				h.log.Infof("Received %s: %s", vf.settlementKey, settleFile.Filename)
			}
		}
	}
	
	// Check if at least one vendor file is provided
	vendors := req.GetVendorFiles()
	if len(vendors) == 0 {
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "At least one vendor file (recon or settlement) is required",
		})
		return
	}
	
	h.log.Infof("Processing %d vendors", len(vendors))
	
	// Process reconciliation
	result, err := h.service.ProcessReconciliation(req)
	if err != nil {
		h.log.Errorf("Reconciliation processing failed: %v", err)
		c.JSON(http.StatusInternalServerError, dto.APIResponse{
			Success: false,
			Message: "Reconciliation processing failed",
			Error:   err.Error(),
		})
		return
	}
	
	h.log.Infof("Reconciliation completed successfully: Job ID %s", result.JobID)
	
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Reconciliation completed successfully",
		Data:    result,
	})
}

// DownloadResult handles CSV download
func (h *ReconciliationHandler) DownloadResult(c *gin.Context) {
	jobID := c.Param("jobId")
	filename := c.Param("filename")
	
	h.log.Infof("Download request: JobID=%s, Filename=%s", jobID, filename)
	
	if jobID == "" || filename == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Job ID and filename are required",
		})
		return
	}
	
	// Get file path
	filePath, err := h.service.DownloadResult(jobID, filename)
	if err != nil {
		h.log.Errorf("File not found: %v", err)
		c.JSON(http.StatusNotFound, dto.APIResponse{
			Success: false,
			Message: "File not found",
			Error:   err.Error(),
		})
		return
	}
	
	// Set headers for download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv")
	
	c.File(filePath)
}

// GetJobStatus handles getting job status
func (h *ReconciliationHandler) GetJobStatus(c *gin.Context) {
	jobID := c.Param("jobId")
	
	h.log.Infof("Status request for Job ID: %s", jobID)
	
	// For now, just return a simple response
	// In production, you might want to check if job directory exists
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Job found",
		Data: map[string]string{
			"job_id": jobID,
			"status": "completed",
		},
	})
}

// GetResultFolders returns list of available result folders
func (h *ReconciliationHandler) GetResultFolders(c *gin.Context) {
	h.log.Info("Fetching result folders")
	
	folders, err := h.service.GetResultFolders()
	if err != nil {
		h.log.Errorf("Failed to get result folders: %v", err)
		c.JSON(http.StatusInternalServerError, dto.APIResponse{
			Success: false,
			Message: "Failed to retrieve result folders",
			Error:   err.Error(),
		})
		return
	}
	
	h.log.Infof("Found %d result folders", len(folders))
	
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Result folders retrieved successfully",
		Data:    folders,
	})
}

// GetResultData returns parsed CSV data for a specific result file
func (h *ReconciliationHandler) GetResultData(c *gin.Context) {
	jobID := c.Param("jobId")
	vendor := c.Param("vendor")
	resultType := c.Param("type") // "recon" or "settlement"
	
	h.log.Infof("Fetching result data: JobID=%s, Vendor=%s, Type=%s", jobID, vendor, resultType)
	
	if jobID == "" || vendor == "" || resultType == "" {
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Job ID, vendor, and result type are required",
		})
		return
	}
	
	data, err := h.service.GetResultData(jobID, vendor, resultType)
	if err != nil {
		h.log.Errorf("Failed to get result data: %v", err)
		c.JSON(http.StatusNotFound, dto.APIResponse{
			Success: false,
			Message: "Failed to retrieve result data",
			Error:   err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Result data retrieved successfully",
		Data:    data,
	})
}
