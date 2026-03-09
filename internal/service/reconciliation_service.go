package service

import (
	"encoding/csv"
	"encoding/json"
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
//   1. Auto-detect job ID untuk hari ini atau buat baru
//   2. Buat/bersihkan folder job
//   3. Auto-detect vendor dari nama file CORE
//   4. Save file upload
//   5. Extract data CORE & Switching
//   6. Bandingkan data (comparison algorithm)
//   7. Write hasil ke CSV
//   8. Return result dengan download URLs
func (s *ReconciliationService) ProcessReconciliation(req *dto.ReconciliationRequest) (*dto.ReconciliationResult, error) {
	// Step 1: Auto-detect atau gunakan job ID yang ada
	var jobID string
	now := time.Now()
	
	if req.JobID != "" {
		// Mode manual: Gunakan job ID yang sudah ada (re-process mode)
		jobID = req.JobID
		s.log.Infof("Re-processing job rekonsiliasi %s (overwrite mode)", jobID)
	} else {
		// Mode auto: Selalu buat job ID baru agar tidak menimpa hasil sebelumnya
		jobID = s.generateJobID(now)
		s.log.Infof("Creating new job: %s", jobID)
	}
	
	jobDir := filepath.Join(s.resultsDir, jobID)
	
	// Step 2: Buat atau bersihkan direktori untuk job ini
	if _, err := os.Stat(jobDir); err == nil {
		// Folder sudah ada, hapus isinya untuk overwrite
		s.log.Infof("Folder job %s sudah ada, membersihkan isi folder...", jobID)
		if err := os.RemoveAll(jobDir); err != nil {
			return nil, fmt.Errorf("gagal menghapus folder job lama: %w", err)
		}
	}
	
	// Buat folder baru
	if err := os.MkdirAll(jobDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("gagal membuat direktori job: %w", err)
	}
	
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
	
	// Calculate totals - Total Records = Sum of all Settlement Total (Match + Mismatch)
	totalRecords := 0
	for _, vr := range results {
		// Total records = total settlement yang diproses (match + mismatch)
		totalRecords += vr.SettlementMatchCount + vr.SettlementMismatchCount
	}
	
	// Generate duplicate report automatically
	s.log.Infof("🔍 Generating duplicate report for job %s...", jobID)
	duplicateReport, err := s.GenerateDuplicateReport(jobID)
	if err != nil {
		s.log.Warnf("Failed to generate duplicate report: %v", err)
		// Don't fail the entire process, just log warning
	} else {
		// Export duplicate report to CSV
		_, exportErr := s.ExportDuplicateReportToCSV(jobID, duplicateReport)
		if exportErr != nil {
			s.log.Warnf("Failed to export duplicate report to CSV: %v", exportErr)
		} else {
			s.log.Infof("✅ Duplicate report generated: %d unique duplicates, %d total records", 
				duplicateReport.TotalDuplicates, duplicateReport.TotalRecords)
		}
	}
	
	// Build result
	reconResult := &dto.ReconciliationResult{
		JobID:         jobID,
		Status:        "completed",
		Message:       "Rekonsiliasi selesai",
		ProcessedAt:   time.Now(),
		TotalRecords:  totalRecords,
		Vendors:       results,
		DownloadURLs:  s.buildDownloadURLsMulti(jobID, vendorFilesMap),
		DuplicateReport: duplicateReport,
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
	pattern := regexp.MustCompile(`^(\d{4})-\d{2}-\d{2}-\d{4}$`)  // format: XXXX-DD-MM-YYYY
	
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

// findTodayJobID mencari job ID yang sudah ada untuk tanggal hari ini
// Format job ID: XXXX-DD-MM-YYYY
// Return: job ID jika ditemukan, string kosong jika tidak ada
func (s *ReconciliationService) findTodayJobID(t time.Time) string {
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return ""
	}
	
	// Format tanggal hari ini: DD-MM-YYYY
	todayDateStr := t.Format("02-01-2006")
	pattern := regexp.MustCompile(`^(\d{4})-` + regexp.QuoteMeta(todayDateStr) + `$`)
	
	// Cari folder dengan tanggal hari ini
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		if pattern.MatchString(entry.Name()) {
			s.log.Infof("Found existing job for today: %s", entry.Name())
			return entry.Name()
		}
	}
	
	return ""
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
		
		// Generate Excel hasil rekonsiliasi dengan sheets terpisah
		resultExcelPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_result.xlsx", vf.Vendor))
		if err := WriteReconResultExcel(resultExcelPath, oldResults); err != nil {
			s.log.Errorf("Failed to write recon result Excel: %v", err)
		} else {
			s.log.Infof("Generated recon result Excel: %s", resultExcelPath)
		}
		
		// Tetap generate CSV untuk backward compatibility
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
		
		// Compare with CORE (sesuai logic Ciptami: hanya return UNMATCHED + match count)
		oldResults, matchCount := CompareSettlementRRNs(coreData, allSettlementData)
		result.SettlementResults = s.convertSettlementResults(oldResults)
		
		// Set match count dari hasil comparison
		result.SettlementMatchCount = matchCount
		
		// Hitung mismatch count
		onlyInCoreCount := s.countOnlyInCore(result.SettlementResults)
		onlyInSwitchingCount := s.countOnlyInSwitching(result.SettlementResults)
		result.SettlementMismatchCount = onlyInCoreCount + onlyInSwitchingCount
		
		// Generate Excel hasil settlement dengan sheets terpisah
		resultExcelPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_result.xlsx", vf.Vendor))
		if err := WriteSettlementResultExcel(resultExcelPath, oldResults); err != nil {
			s.log.Errorf("Failed to write settlement result Excel: %v", err)
		} else {
			s.log.Infof("Generated settlement result Excel: %s", resultExcelPath)
		}
		
		// Tetap generate CSV untuk backward compatibility
		resultCSVPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_result.csv", vf.Vendor))
		if err := WriteSettlementResultCSV(resultCSVPath, oldResults); err != nil {
			s.log.Errorf("Failed to write settlement result CSV: %v", err)
		} else {
			s.log.Infof("Generated settlement result CSV: %s", resultCSVPath)
		}
		
		// Save metadata JSON untuk settlement
		metadataPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_metadata.json", vf.Vendor))
		if err := s.saveMetadata(metadataPath, map[string]int{
			"match_count":    result.SettlementMatchCount,
			"mismatch_count": result.SettlementMismatchCount,
			"only_in_core":   onlyInCoreCount,
			"only_in_switching": onlyInSwitchingCount,
		}); err != nil {
			s.log.Errorf("Failed to write settlement metadata: %v", err)
		} else {
			s.log.Infof("Generated settlement metadata: %s", metadataPath)
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
			MerchantName:     r.MerchantName,
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
			MerchantName:     r.MerchantName,
			SettlementAmount: fmt.Sprintf("%.2f", r.Amount),
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

// countOnlyInCore menghitung jumlah record ONLY_IN_CORE
func (s *ReconciliationService) countOnlyInCore(results []dto.SettlementData) int {
	count := 0
	for _, r := range results {
		if r.MatchStatus == "ONLY_IN_CORE" {
			count++
		}
	}
	return count
}

// countOnlyInSwitching menghitung jumlah record ONLY_IN_SWITCHING
func (s *ReconciliationService) countOnlyInSwitching(results []dto.SettlementData) int {
	count := 0
	for _, r := range results {
		if r.MatchStatus == "ONLY_IN_SWITCHING" {
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
	// Add duplicate report URL
	urls["duplicate_report"] = fmt.Sprintf("/api/download/%s/%s_duplicate_report.csv", jobID, jobID)
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
	var result interface{}
	if resultType == "recon" {
		result, err = s.parseReconResults(records)
	} else if resultType == "settlement" {
		result, err = s.parseSettlementResults(records)
		if err == nil {
			// Load metadata for settlement
			metadataPath := filepath.Join(s.resultsDir, jobID, fmt.Sprintf("%s_settlement_metadata.json", vendor))
			metadata, metaErr := s.loadMetadata(metadataPath)
			if metaErr == nil {
				resultMap := result.(map[string]interface{})
				resultMap["metadata"] = metadata
			}
		}
	} else {
		return nil, fmt.Errorf("invalid result type: %s", resultType)
	}
	
	return result, err
}

// parseReconResults parses reconciliation result CSV
// CSV Format: No, RRN, Reff, Status, Match Status, Merchant PAN, Merchant Name, Merchant Criteria, Invoice Number, Created Date, Created Time, Processing Code
func (s *ReconciliationService) parseReconResults(records [][]string) ([]map[string]interface{}, error) {
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
		
		// Add source field based on match_status
		source := "BOTH"
		matchStatus := row[4]
		if matchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if matchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[1],
			"reff":              row[2],
			"status":            row[3],
			"match_status":      row[4],
			"source":            source,
			"merchant_pan":      row[5],
			"merchant_name":     row[6],
			"merchant_criteria": row[7],
			"invoice_number":    row[8],
			"created_date":      row[9],
			"created_time":      row[10],
			"process_code":      row[11],
		})
	}
	
	return results, nil
}

// parseSettlementResults parses settlement result CSV
// CSV Format: No, RRN, Amount, Reff, Status, Match Status, Merchant PAN, Merchant Name, Merchant Criteria, Invoice Number, Created Date, Created Time, Processing Code, Interchange Fee, Convenience Fee
func (s *ReconciliationService) parseSettlementResults(records [][]string) (map[string]interface{}, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("no data rows in CSV")
	}
	
	// Skip header row
	var results []map[string]interface{}
	
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 15 {
			continue
		}
		
		// Parse amount (column index 2, after No column)
		var amount float64
		if row[2] != "" {
			parsedAmount, err := strconv.ParseFloat(row[2], 64)
			if err == nil {
				amount = parsedAmount
			}
		}
		
		// Add source field based on match_status
		source := "BOTH"
		matchStatus := row[5]
		if matchStatus == "MATCH" {
			source = "BOTH"
		} else if matchStatus == "ONLY_IN_CORE" {
			source = "CORE"
		} else if matchStatus == "ONLY_IN_SWITCHING" {
			source = "SWITCHING"
		}
		
		results = append(results, map[string]interface{}{
			"rrn":               row[1],
			"settlement_amount": fmt.Sprintf("%.2f", amount),
			"reff":              row[3],
			"status":            row[4],
			"match_status":      row[5],
			"source":            source,
			"merchant_pan":      row[6],
			"merchant_name":     row[7],
			"interchange_fee":   row[13],
			"convenience_fee":   row[14],
		})
	}
	
	// Return dengan metadata placeholder (akan di-override oleh loadMetadata)
	return map[string]interface{}{
		"data": results,
		"metadata": map[string]int{
			"match_count":    0,
			"mismatch_count": 0,
			"only_in_core":   0,
			"only_in_switching": 0,
		},
	}, nil
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
	
	s.log.Infof("📂 Scanning job directory: %s", jobDir)
	
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
			s.log.Infof("✅ Found CORE file: %s", filename)
			continue
		}
		
		// RECON file pattern: *_recon_*.csv (exclude *_result.csv)
		if matched, _ := filepath.Match("*_recon_*.csv", filename); matched {
			if matched2, _ := filepath.Match("*_result.csv", filename); !matched2 {
				// Extract vendor name from filename (e.g., "alto_recon_0.csv" -> "alto")
				vendor := extractVendorFromFilename(filename)
				if vendor != "" {
					reconPaths[vendor] = fullPath
					s.log.Infof("✅ Found RECON file for %s: %s", vendor, filename)
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
					s.log.Infof("✅ Found SETTLEMENT file for %s: %s", vendor, filename)
				}
			}
			continue
		}
	}
	
	// Log summary
	s.log.Infof("📊 Files found - CORE: %v, RECON: %d vendors, SETTLEMENT: %d vendors", 
		corePath != "", len(reconPaths), len(settlePaths))
	
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

// GetResultsDir returns the results directory path
func (s *ReconciliationService) GetResultsDir() string {
	return s.resultsDir
}

// saveMetadata saves metadata to JSON file
func (s *ReconciliationService) saveMetadata(filePath string, metadata map[string]int) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}
	
	return nil
}

// loadMetadata loads metadata from JSON file
func (s *ReconciliationService) loadMetadata(filePath string) (map[string]int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}
	
	var metadata map[string]int
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	
	return metadata, nil
}

