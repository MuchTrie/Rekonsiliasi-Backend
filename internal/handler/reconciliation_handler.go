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
	
	// Parse multipart form (allow larger files for multi-file upload)
	if err := c.Request.ParseMultipartForm(500 << 20); err != nil { // 500 MB max
		h.log.Errorf("Failed to parse multipart form: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Failed to parse request",
			Error:   err.Error(),
		})
		return
	}
	
	// Get multiple core files
	form := c.Request.MultipartForm
	coreFiles := form.File["core_files"]
	
	if len(coreFiles) == 0 {
		h.log.Error("At least one core file is required")
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "At least one core file is required",
			Error:   "No core files uploaded",
		})
		return
	}
	
	// Validate core files
	for i, coreFile := range coreFiles {
		if err := h.validator.ValidateCSVFile(coreFile); err != nil {
			h.log.Errorf("Core file %d (%s) validation failed: %v", i, coreFile.Filename, err)
			c.JSON(http.StatusBadRequest, dto.APIResponse{
				Success: false,
				Message: fmt.Sprintf("Core file validation failed: %s", coreFile.Filename),
				Error:   err.Error(),
			})
			return
		}
	}
	
	h.log.Infof("Received %d core file(s)", len(coreFiles))
	
	// Build request
	req := &dto.ReconciliationRequest{
		CoreFiles: coreFiles,
	}
	
	// Get vendor files (multi-file support)
	vendorFileKeys := []struct {
		reconKey      string
		settlementKey string
		reconDest     *[]*multipart.FileHeader
		settleDest    *[]*multipart.FileHeader
	}{
		{"alto_recon_files", "alto_settlement_files", &req.AltoReconFiles, &req.AltoSettlementFiles},
		{"jalin_recon_files", "jalin_settlement_files", &req.JalinReconFiles, &req.JalinSettlementFiles},
		{"aj_recon_files", "aj_settlement_files", &req.AJReconFiles, &req.AJSettlementFiles},
		{"rinti_recon_files", "rinti_settlement_files", &req.RintiReconFiles, &req.RintiSettlementFiles},
	}
	
	for _, vfk := range vendorFileKeys {
		// Recon files
		if reconFiles := form.File[vfk.reconKey]; len(reconFiles) > 0 {
			validReconFiles := []*multipart.FileHeader{}
			for _, reconFile := range reconFiles {
				// Allow any file format for recon files (vendor specific format)
				if err := h.validator.ValidateReconFile(reconFile); err != nil {
					h.log.Warnf("Skipping %s file %s: %v", vfk.reconKey, reconFile.Filename, err)
				} else {
					validReconFiles = append(validReconFiles, reconFile)
					h.log.Infof("Received %s: %s", vfk.reconKey, reconFile.Filename)
				}
			}
			*vfk.reconDest = validReconFiles
		}
		
		// Settlement files - use ValidateSettlementFile which accepts any extension
		if settleFiles := form.File[vfk.settlementKey]; len(settleFiles) > 0 {
			h.log.Infof("Found %d settlement file(s) for %s", len(settleFiles), vfk.settlementKey)
			validSettleFiles := []*multipart.FileHeader{}
			for _, settleFile := range settleFiles {
				// Settlement files can have any extension, use special validator
				if err := h.validator.ValidateSettlementFile(settleFile); err != nil {
					h.log.Warnf("Skipping %s file %s: %v", vfk.settlementKey, settleFile.Filename, err)
				} else {
					validSettleFiles = append(validSettleFiles, settleFile)
					h.log.Infof("Received %s: %s (size: %d bytes)", vfk.settlementKey, settleFile.Filename, settleFile.Size)
				}
			}
			*vfk.settleDest = validSettleFiles
		}
	}
	
	// Check if at least one vendor can be processed
	vendorMap := req.GetVendorFilesMap()
	if len(vendorMap) == 0 {
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "No valid vendor detected. Please ensure core files contain vendor name (ALTO/JALIN/AJ/RINTI) and at least one vendor has recon or settlement files.",
		})
		return
	}
	
	h.log.Infof("Processing %d vendor(s)", len(vendorMap))
	
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

// ConvertSettlement handles settlement file conversion from TXT to CSV
func (h *ReconciliationHandler) ConvertSettlement(c *gin.Context) {
	h.log.Info("Received settlement conversion request")
	
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(100 << 20); err != nil { // 100 MB max
		h.log.Errorf("Failed to parse multipart form: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Failed to parse request",
			Error:   err.Error(),
		})
		return
	}
	
	// Get settlement file
	file, err := c.FormFile("settlement_file")
	if err != nil {
		h.log.Errorf("Failed to get settlement file: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Settlement file is required",
			Error:   err.Error(),
		})
		return
	}
	
	// Validate settlement file (accepts any file format, will be processed as TXT)
	if err := h.validator.ValidateSettlementFile(file); err != nil {
		h.log.Errorf("Settlement file validation failed: %v", err)
		c.JSON(http.StatusBadRequest, dto.APIResponse{
			Success: false,
			Message: "Invalid settlement file",
			Error:   err.Error(),
		})
		return
	}
	
	h.log.Infof("Converting settlement file: %s (size: %d bytes)", file.Filename, file.Size)
	
	// Convert settlement file
	result, err := h.service.ConvertSettlementFile(file)
	if err != nil {
		h.log.Errorf("Settlement conversion failed: %v", err)
		c.JSON(http.StatusInternalServerError, dto.APIResponse{
			Success: false,
			Message: "Settlement conversion failed",
			Error:   err.Error(),
		})
		return
	}
	
	h.log.Infof("Settlement conversion completed: %s (%d records)", result.Filename, result.TotalRecords)
	
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Settlement file converted successfully",
		Data:    result,
	})
}

// GetConvertedFiles returns list of previously converted settlement files
func (h *ReconciliationHandler) GetConvertedFiles(c *gin.Context) {
	h.log.Info("Fetching converted settlement files")
	
	files, err := h.service.GetConvertedFiles()
	if err != nil {
		h.log.Errorf("Failed to get converted files: %v", err)
		c.JSON(http.StatusInternalServerError, dto.APIResponse{
			Success: false,
			Message: "Failed to retrieve converted files",
			Error:   err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, dto.APIResponse{
		Success: true,
		Message: "Converted files retrieved successfully",
		Data:    files,
	})
}
