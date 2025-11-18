package service

import (
	"encoding/csv"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ============================================================================
// RECONCILIATION SERVICE - STRUCT & CONSTRUCTOR
// ============================================================================

// ReconciliationService adalah service utama yang mengatur seluruh proses rekonsiliasi
// Bertanggung jawab untuk:
// - Mengelola upload file
// - Koordinasi proses ekstraksi data
// - Menjalankan algoritma perbandingan
// - Menyimpan hasil ke CSV
// - Generate job ID dan download URLs
type ReconciliationService struct {
	log                 *logrus.Logger           // Logger untuk tracking
	uploadDir           string                   // Direktori untuk file upload sementara
	resultsDir          string                   // Direktori untuk hasil rekonsiliasi
	jobCounter          int                      // Counter untuk job ID (4 digit)
	mu                  sync.Mutex               // Mutex untuk thread safety
	fileConverter       *FileConverter           // Converter TXT → CSV
	dataExtractor       *DataExtractor           // Extractor data dari file
	settlementConverter *SettlementConverter     // Converter settlement khusus
	duplicateDetector   *DuplicateDetector       // Duplicate RRN detector
}

// NewReconciliationService membuat instance baru dari ReconciliationService
// Parameter:
//   - log: Logger untuk tracking proses
//   - uploadDir: Path direktori untuk upload temporary
//   - resultsDir: Path direktori untuk menyimpan hasil
func NewReconciliationService(log *logrus.Logger, uploadDir, resultsDir string) *ReconciliationService {
	return &ReconciliationService{
		log:                 log,
		uploadDir:           uploadDir,
		resultsDir:          resultsDir,
		jobCounter:          0, // Counter akan di-load dari folder yang sudah ada
		fileConverter:       NewFileConverter(log),
		dataExtractor:       NewDataExtractor(log),
		settlementConverter: NewSettlementConverter(log, uploadDir, resultsDir),
		duplicateDetector:   NewDuplicateDetector(log),
	}
}

// ============================================================================
// FUNGSI UTAMA - PROCESS RECONCILIATION
// ============================================================================

// ProcessReconciliation adalah fungsi utama yang memproses rekonsiliasi dari file upload
// Flow:
//   1. Generate job ID (XXXX-DD-MM-YYYY)
//   2. Buat folder job
//   3. Auto-detect vendor dari nama file CORE
//   4. Save file upload
//   5. Extract data CORE & Switching
//   6. Bandingkan data (comparison algorithm)
//   7. Write hasil ke CSV
//   8. Return result dengan download URLs
func (s *ReconciliationService) ProcessReconciliation(req *dto.ReconciliationRequest) (*dto.ReconciliationResult, error) {
	// Step 1: Generate job ID dengan format XXXX-DD-MM-YYYY
	now := time.Now()
	jobID := s.generateJobID(now)
	jobDir := filepath.Join(s.resultsDir, jobID)
	
	// Step 2: Buat direktori untuk job ini
	if err := os.MkdirAll(jobDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("gagal membuat direktori job: %w", err)
	}
	
	s.log.Infof("Memproses job rekonsiliasi %s", jobID)
	
	// Step 3: Dapatkan map vendor files (auto-detect dari nama file CORE)
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
		
		file, err := os.Open(csvPath)
		if err != nil {
			s.log.Errorf("Failed to open %s settlement CSV %d: %v", vf.Vendor, idx, err)
			continue
		}
		
		settlementData := s.dataExtractor.ExtractSettlementDataFromCSV(file)
		file.Close()			// Merge data
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
		
		// Add source field based on match_status
		source := "BOTH"
		matchStatus := row[3]
		if matchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if matchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[0],
			"reff":              row[1],
			"status":            row[2],
			"match_status":      row[3],
			"source":            source,
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
		
		// Parse amount (column index 1)
		var amount float64
		if row[1] != "" {
			parsedAmount, err := strconv.ParseFloat(row[1], 64)
			if err == nil {
				amount = parsedAmount
			}
		}
		
		// Add source field based on match_status
		source := "BOTH"
		matchStatus := row[4]
		if matchStatus == "MATCH" {
			source = "BOTH"
		} else if matchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if matchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[0],
			"amount":            amount,
			"reff":              row[2],
			"status":            row[3],
			"match_status":      row[4],
			"source":            source,
			"merchant_pan":      row[5],
			"interchange_fee":   row[11],
			"convenience_fee":   row[12],
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

// PreviewConvertedFile delegates to SettlementConverter
func (s *ReconciliationService) PreviewConvertedFile(filename string) (*dto.SettlementConversionResult, error) {
	return s.settlementConverter.PreviewConvertedFile(filename)
}

// ============================================================================
// DUPLICATE DETECTION
// ============================================================================

// GenerateDuplicateReport generates a complete duplicate detection report for a job
func (s *ReconciliationService) GenerateDuplicateReport(jobID string) (*dto.DuplicateReport, error) {
	s.log.Infof("Generating duplicate report for job: %s", jobID)
	
	jobDir := filepath.Join(s.resultsDir, jobID)
	
	// Check if job directory exists
	if _, err := os.Stat(jobDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("job directory not found: %s", jobID)
	}
	
	// Build file paths maps
	corePath := ""
	reconPaths := make(map[string]string)
	settlePaths := make(map[string]string)
	
	// Read directory to find files
	files, err := os.ReadDir(jobDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read job directory: %w", err)
	}
	
	// Identify files by pattern
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		
		filename := file.Name()
		fullPath := filepath.Join(jobDir, filename)
		
		// CORE file pattern: *_core.csv
		if matched, _ := filepath.Match("*_core.csv", filename); matched {
			corePath = fullPath
			continue
		}
		
		// RECON file pattern: *_recon_*.csv (exclude *_result.csv)
		if matched, _ := filepath.Match("*_recon_*.csv", filename); matched {
			if matched2, _ := filepath.Match("*_result.csv", filename); !matched2 {
				// Extract vendor name from filename (e.g., "alto_recon_0.csv" -> "alto")
				vendor := extractVendorFromFilename(filename)
				if vendor != "" {
					reconPaths[vendor] = fullPath
				}
			}
			continue
		}
		
		// SETTLEMENT file pattern: *_settlement_*.csv (exclude *_result.csv)
		if matched, _ := filepath.Match("*_settlement_*.csv", filename); matched {
			if matched2, _ := filepath.Match("*_result.csv", filename); !matched2 {
				vendor := extractVendorFromFilename(filename)
				if vendor != "" {
					settlePaths[vendor] = fullPath
				}
			}
			continue
		}
	}
	
	// Generate report
	report, err := s.duplicateDetector.GenerateDuplicateReport(jobID, corePath, reconPaths, settlePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to generate duplicate report: %w", err)
	}
	
	return report, nil
}

// ExportDuplicateReportToCSV exports duplicate report to CSV file
func (s *ReconciliationService) ExportDuplicateReportToCSV(jobID string, report *dto.DuplicateReport) (string, error) {
	s.log.Infof("Exporting duplicate report to CSV for job: %s", jobID)
	
	jobDir := filepath.Join(s.resultsDir, jobID)
	outputPath := filepath.Join(jobDir, fmt.Sprintf("%s_duplicate_report.csv", jobID))
	
	err := s.duplicateDetector.ExportDuplicatesToCSV(report, outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to export duplicate report: %w", err)
	}
	
	return outputPath, nil
}

// extractVendorFromFilename extracts vendor name from filename
// Example: "alto_recon_0.csv" -> "alto", "jalin_settlement_0.csv" -> "jalin"
func extractVendorFromFilename(filename string) string {
	vendors := []string{"alto", "jalin", "aj", "rinti"}
	filenameLower := filepath.Base(filename)
	filenameLower = filepath.Ext(filenameLower) // Remove extension
	filenameLower = filename[:len(filename)-len(filepath.Ext(filename))]
	
	for _, vendor := range vendors {
		if len(filenameLower) >= len(vendor) && filenameLower[:len(vendor)] == vendor {
			return vendor
		}
	}
	
	return ""
}

