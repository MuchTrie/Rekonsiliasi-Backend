package service

import (
	"encoding/csv"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ReconciliationService handles reconciliation business logic
type ReconciliationService struct {
	log                 *logrus.Logger
	uploadDir           string
	resultsDir          string
	jobCounter          int // Counter for 4-digit ID
	mu                  sync.Mutex
	fileConverter       *FileConverter
	dataExtractor       *DataExtractor
	settlementConverter *SettlementConverter
}

// NewReconciliationService creates a new reconciliation service
func NewReconciliationService(log *logrus.Logger, uploadDir, resultsDir string) *ReconciliationService {
	return &ReconciliationService{
		log:                 log,
		uploadDir:           uploadDir,
		resultsDir:          resultsDir,
		jobCounter:          0, // Will be loaded from existing folders
		fileConverter:       NewFileConverter(log),
		dataExtractor:       NewDataExtractor(log),
		settlementConverter: NewSettlementConverter(log, uploadDir, resultsDir),
	}
}

// ProcessReconciliation memproses rekonsiliasi dari file yang diupload
func (s *ReconciliationService) ProcessReconciliation(req *dto.ReconciliationRequest) (*dto.ReconciliationResult, error) {
	// Generate job ID dengan format: XXXX-DD-MM-YYYY
	now := time.Now()
	jobID := s.generateJobID(now)
	jobDir := filepath.Join(s.resultsDir, jobID)
	
	// Buat direktori untuk job ini
	if err := os.MkdirAll(jobDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create job directory: %w", err)
	}
	
	s.log.Infof("Processing reconciliation job %s", jobID)
	
	// Get vendor files map (auto-detect vendor from CORE filenames)
	vendorFilesMap := req.GetVendorFilesMap()
	
	if len(vendorFilesMap) == 0 {
		return nil, fmt.Errorf("no valid vendor detected from CORE files. Please include vendor name (ALTO/JALIN/AJ/RINTI) in filename")
	}
	
	// Process each vendor
	var wg sync.WaitGroup
	results := make([]dto.VendorResult, 0, len(vendorFilesMap))
	resultsMu := sync.Mutex{}
	
	for vendor, vf := range vendorFilesMap {
		wg.Add(1)
		go func(vendorName string, vendorFile *dto.VendorFiles) {
			defer wg.Done()
			
			result := s.processVendorMultiFile(vendorFile, jobDir)
			
			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()
		}(vendor, vf)
	}
	
	wg.Wait()
	
	// Calculate totals
	totalRecords := 0
	for _, vr := range results {
		totalRecords += len(vr.ReconResults) + len(vr.SettlementResults)
	}
	
	// Build result
	reconResult := &dto.ReconciliationResult{
		JobID:        jobID,
		Status:       "completed",
		Message:      "Rekonsiliasi selesai",
		ProcessedAt:  time.Now(),
		TotalRecords: totalRecords,
		Vendors:      results,
		DownloadURLs: s.buildDownloadURLsMulti(jobID, vendorFilesMap),
	}
	
	s.log.Infof("Job %s completed with %d total records", jobID, totalRecords)
	
	return reconResult, nil
}

// generateJobID generates job ID in format: XXXX-DD-MM-YYYY
func (s *ReconciliationService) generateJobID(t time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Load max ID from existing folders
	if s.jobCounter == 0 {
		s.jobCounter = s.getLastJobID()
	}
	
	s.jobCounter++
	
	return fmt.Sprintf("%04d-%s", s.jobCounter, t.Format("02-01-2006"))
}

// getLastJobID reads existing result folders and returns the highest ID
func (s *ReconciliationService) getLastJobID() int {
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return 0
	}
	
	maxID := 0
	pattern := regexp.MustCompile(`^(\d{4})-\d{2}-\d{2}-\d{4}$`)
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		matches := pattern.FindStringSubmatch(entry.Name())
		if len(matches) > 1 {
			var id int
			fmt.Sscanf(matches[1], "%d", &id)
			if id > maxID {
				maxID = id
			}
		}
	}
	
	return maxID
}

// processVendorMultiFile memproses satu vendor dengan multi-file support
func (s *ReconciliationService) processVendorMultiFile(vf *dto.VendorFiles, jobDir string) dto.VendorResult {
	result := dto.VendorResult{
		Vendor: vf.Vendor,
	}
	
	// 1. Extract CORE data for this vendor
	var coreData []*dto.Data
	if vf.CoreFile != nil {
		corePath := filepath.Join(jobDir, fmt.Sprintf("%s_core.csv", vf.Vendor))
		if err := saveUploadedFile(vf.CoreFile, corePath); err != nil {
			s.log.Errorf("Failed to save %s core file: %v", vf.Vendor, err)
			return result
		}
		
		var err error
		coreData, err = s.dataExtractor.ExtractSingleCoreData(corePath)
		if err != nil {
			s.log.Errorf("Failed to extract %s core data: %v", vf.Vendor, err)
			return result
		}
		
		s.log.Infof("Loaded %d core records for vendor %s", len(coreData), vf.Vendor)
	}
	
	// 2. Process multiple Recon files
	if len(vf.ReconFiles) > 0 {
		allReconData := make(map[string]dto.SwitchingReconciliationData)
		
		for idx, reconFile := range vf.ReconFiles {
			reconPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_%d.txt", vf.Vendor, idx))
			if err := saveUploadedFile(reconFile, reconPath); err != nil {
				s.log.Errorf("Failed to save %s recon file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			// Convert TXT to CSV
			csvPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_%d.csv", vf.Vendor, idx))
			if err := s.fileConverter.ConvertReconTxtToCsv(reconPath, csvPath); err != nil {
				s.log.Errorf("Failed to convert %s recon file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			file, err := os.Open(csvPath)
			if err != nil {
				s.log.Errorf("Failed to open %s recon CSV %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			reconData := s.dataExtractor.ExtractReconciliationDataNew(file)
			file.Close()
			
			// Merge data
			for rrn, data := range reconData {
				allReconData[rrn] = data
			}
		}
		
		s.log.Infof("Loaded total %d recon records for vendor %s", len(allReconData), vf.Vendor)
		
		// Compare with CORE
		oldResults := CompareReconRRNs(coreData, allReconData)
		result.ReconResults = s.convertReconResults(oldResults)
		result.ReconMatchCount = s.countMatches(result.ReconResults)
		result.ReconMismatchCount = len(result.ReconResults) - result.ReconMatchCount
		
		// Generate CSV hasil rekonsiliasi
		resultCSVPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_result.csv", vf.Vendor))
		if err := WriteReconResultCSV(resultCSVPath, oldResults); err != nil {
			s.log.Errorf("Failed to write recon result CSV: %v", err)
		} else {
			s.log.Infof("Generated recon result CSV: %s", resultCSVPath)
		}
	}
	
	// 3. Process multiple Settlement files
	if len(vf.SettlementFiles) > 0 {
		allSettlementData := make(map[string]dto.SwitchingSettlementData)
		
		for idx, settlementFile := range vf.SettlementFiles {
			settlementPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_%d.txt", vf.Vendor, idx))
			if err := saveUploadedFile(settlementFile, settlementPath); err != nil {
				s.log.Errorf("Failed to save %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			// Convert TXT to CSV
			csvPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_%d.csv", vf.Vendor, idx))
			if err := s.fileConverter.ConvertSettlementTxtToCsv(settlementPath, csvPath); err != nil {
				s.log.Errorf("Failed to convert %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			file, err := os.Open(settlementPath)
			if err != nil {
				s.log.Errorf("Failed to open %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			settlementData := s.dataExtractor.ExtractSettlementData(file)
			file.Close()
			
			// Merge data
			for rrn, data := range settlementData {
				allSettlementData[rrn] = data
			}
		}
		
		s.log.Infof("Loaded total %d settlement records for vendor %s", len(allSettlementData), vf.Vendor)
		
		// Compare with CORE
		oldResults := CompareSettlementRRNs(coreData, allSettlementData)
		result.SettlementResults = s.convertSettlementResults(oldResults)
		result.SettlementMatchCount = s.countMatchesSettlement(result.SettlementResults)
		result.SettlementMismatchCount = len(result.SettlementResults) - result.SettlementMatchCount
		
		// Generate CSV hasil settlement
		resultCSVPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_result.csv", vf.Vendor))
		if err := WriteSettlementResultCSV(resultCSVPath, oldResults); err != nil {
			s.log.Errorf("Failed to write settlement result CSV: %v", err)
		} else {
			s.log.Infof("Generated settlement result CSV: %s", resultCSVPath)
		}
	}
	
	return result
}

// convertReconResults converts old DTO to web DTO
func (s *ReconciliationService) convertReconResults(old []dto.ReconciliationSwitchingResult) []dto.ReconciliationData {
	results := make([]dto.ReconciliationData, len(old))
	for i, r := range old {
		source := "BOTH"
		if r.MatchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if r.MatchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results[i] = dto.ReconciliationData{
			RRN:              r.RRN,
			Reff:             r.Reff,
			Status:           r.Status,
			MatchStatus:      r.MatchStatus,
			Source:           source,
			MerchantPAN:      r.MerchantPAN,
			MerchantCriteria: r.MerchantCriteria,
			InvoiceNumber:    r.InvoiceNumber,
			CreatedDate:      r.CreatedDate,
			CreatedTime:      r.CreatedTime,
			ProcessCode:      r.ProcessingCode,
		}
	}
	return results
}

// convertSettlementResults converts old settlement DTO to web DTO
func (s *ReconciliationService) convertSettlementResults(old []dto.SettlementSwitchingResult) []dto.SettlementData {
	results := make([]dto.SettlementData, len(old))
	for i, r := range old {
		source := "BOTH"
		if r.MatchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if r.MatchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results[i] = dto.SettlementData{
			RRN:              r.RRN,
			Reff:             r.Reff,
			Status:           r.Status,
			MatchStatus:      r.MatchStatus,
			Source:           source,
			MerchantPAN:      r.MerchantPAN,
			SettlementAmount: "",
			InterchangeFee:   r.InterchangeFee,
			ConvenienceFee:   r.ConvenienceFee,
		}
	}
	return results
}

// countMatches menghitung jumlah match
func (s *ReconciliationService) countMatches(results []dto.ReconciliationData) int {
	count := 0
	for _, r := range results {
		if r.MatchStatus == "MATCH" {
			count++
		}
	}
	return count
}

// countMatchesSettlement menghitung jumlah match settlement
func (s *ReconciliationService) countMatchesSettlement(results []dto.SettlementData) int {
	count := 0
	for _, r := range results {
		if r.MatchStatus == "MATCH" {
			count++
		}
	}
	return count
}

// buildDownloadURLsMulti membuat URL download untuk hasil multi-file
func (s *ReconciliationService) buildDownloadURLsMulti(jobID string, vendorFilesMap map[string]*dto.VendorFiles) map[string]string {
	urls := make(map[string]string)
	for vendor, vf := range vendorFilesMap {
		if len(vf.ReconFiles) > 0 {
			urls[vendor+"_recon_result"] = fmt.Sprintf("/api/download/%s/%s_recon_result.csv", jobID, vendor)
		}
		if len(vf.SettlementFiles) > 0 {
			urls[vendor+"_settlement_result"] = fmt.Sprintf("/api/download/%s/%s_settlement_result.csv", jobID, vendor)
		}
	}
	return urls
}

// DownloadResult mendownload hasil CSV
func (s *ReconciliationService) DownloadResult(jobID, filename string) (string, error) {
	filePath := filepath.Join(s.resultsDir, jobID, filename)
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", filename)
	}
	
	return filePath, nil
}

// GetResultFolders returns list of available result folders with their files
func (s *ReconciliationService) GetResultFolders() ([]map[string]interface{}, error) {
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read results directory: %w", err)
	}
	
	var folders []map[string]interface{}
	
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "converted" {
			continue
		}
		
		// Read files in folder
		folderPath := filepath.Join(s.resultsDir, entry.Name())
		files, err := os.ReadDir(folderPath)
		if err != nil {
			s.log.Warnf("Failed to read folder %s: %v", entry.Name(), err)
			continue
		}
		
		fileNames := make([]string, 0)
		for _, file := range files {
			if !file.IsDir() {
				fileNames = append(fileNames, file.Name())
			}
		}
		
		folders = append(folders, map[string]interface{}{
			"name":  entry.Name(),
			"files": fileNames,
		})
	}
	
	return folders, nil
}

// GetResultData reads and parses result CSV file
func (s *ReconciliationService) GetResultData(jobID, vendor, resultType string) (interface{}, error) {
	// Build filename
	filename := fmt.Sprintf("%s_%s_result.csv", vendor, resultType)
	filePath := filepath.Join(s.resultsDir, jobID, filename)
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", filename)
	}
	
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	// Parse CSV based on type
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	
	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}
	
	if len(records) == 0 {
		return nil, fmt.Errorf("empty CSV file")
	}
	
	// Parse based on result type
	if resultType == "recon" {
		return s.parseReconResults(records)
	} else if resultType == "settlement" {
		return s.parseSettlementResults(records)
	}
	
	return nil, fmt.Errorf("invalid result type: %s", resultType)
}

// parseReconResults parses reconciliation result CSV
func (s *ReconciliationService) parseReconResults(records [][]string) ([]map[string]interface{}, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("no data rows in CSV")
	}
	
	// Skip header row
	var results []map[string]interface{}
	
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 10 {
			continue
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[0],
			"reff":              row[1],
			"status":            row[2],
			"match_status":      row[3],
			"merchant_pan":      row[4],
			"merchant_criteria": row[5],
			"invoice_number":    row[6],
			"created_date":      row[7],
			"created_time":      row[8],
			"process_code":      row[9],
		})
	}
	
	return results, nil
}

// parseSettlementResults parses settlement result CSV
func (s *ReconciliationService) parseSettlementResults(records [][]string) ([]map[string]interface{}, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("no data rows in CSV")
	}
	
	// Skip header row
	var results []map[string]interface{}
	
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 12 {
			continue
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[0],
			"reff":              row[1],
			"status":            row[2],
			"match_status":      row[3],
			"merchant_pan":      row[4],
			"interchange_fee":   row[10],
			"convenience_fee":   row[11],
		})
	}
	
	return results, nil
}

// ConvertSettlementFile delegates to SettlementConverter
func (s *ReconciliationService) ConvertSettlementFile(file *multipart.FileHeader) (*dto.SettlementConversionResult, error) {
	return s.settlementConverter.ConvertSettlementFile(file)
}

// GetConvertedFiles delegates to SettlementConverter
func (s *ReconciliationService) GetConvertedFiles() ([]map[string]interface{}, error) {
	return s.settlementConverter.GetConvertedFiles()
}
