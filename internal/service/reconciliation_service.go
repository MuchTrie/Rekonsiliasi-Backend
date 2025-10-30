package service

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ReconciliationService handles reconciliation business logic
type ReconciliationService struct {
	log        *logrus.Logger
	uploadDir  string
	resultsDir string
}

// NewReconciliationService creates a new reconciliation service
func NewReconciliationService(log *logrus.Logger) *ReconciliationService {
	return &ReconciliationService{
		log:        log,
		uploadDir:  "uploads",
		resultsDir: "results",
	}
}

// ProcessReconciliation memproses rekonsiliasi dari file yang diupload
func (s *ReconciliationService) ProcessReconciliation(req *dto.ReconciliationRequest) (*dto.ReconciliationResult, error) {
	// Generate job ID dengan format tanggal: DDMMYYYY
	now := time.Now()
	jobID := now.Format("02012006") // Format: DDMMYYYY
	jobDir := filepath.Join(s.resultsDir, jobID)
	
	// Buat direktori untuk job ini
	if err := os.MkdirAll(jobDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create job directory: %w", err)
	}
	
	s.log.Infof("Processing reconciliation job %s", jobID)
	
	// 1. Save core file
	coreFilePath := filepath.Join(jobDir, "core_file.csv")
	if err := s.saveUploadedFile(req.CoreFile, coreFilePath); err != nil {
		return nil, fmt.Errorf("failed to save core file: %w", err)
	}
	
	// 2. Extract core data
	coreData, err := s.extractCoreData(coreFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract core data: %w", err)
	}
	
	s.log.Infof("Loaded %d core records", len(coreData))
	
	// 3. Process vendor files
	vendorFiles := req.GetVendorFiles()
	var wg sync.WaitGroup
	results := make([]dto.VendorResult, len(vendorFiles))
	
	for i, vf := range vendorFiles {
		wg.Add(1)
		go func(idx int, vendorFile dto.VendorFiles) {
			defer wg.Done()
			
			vendorData := coreData[vendorFile.Vendor]
			result := s.processVendor(vendorFile, vendorData, jobDir)
			results[idx] = result
		}(i, vf)
	}
	
	wg.Wait()
	
	// 4. Calculate totals
	totalRecords := 0
	for _, vr := range results {
		totalRecords += len(vr.ReconResults) + len(vr.SettlementResults)
	}
	
	// 5. Build result
	reconResult := &dto.ReconciliationResult{
		JobID:        jobID,
		Status:       "completed",
		Message:      "Rekonsiliasi selesai",
		ProcessedAt:  time.Now(),
		TotalRecords: totalRecords,
		Vendors:      results,
		DownloadURLs: s.buildDownloadURLs(jobID, vendorFiles),
	}
	
	s.log.Infof("Job %s completed with %d total records", jobID, totalRecords)
	
	return reconResult, nil
}

// processVendor memproses satu vendor (recon & settlement)
func (s *ReconciliationService) processVendor(vf dto.VendorFiles, coreData []*dto.Data, jobDir string) dto.VendorResult {
	result := dto.VendorResult{
		Vendor: vf.Vendor,
	}
	
	// Process Recon file
	if vf.ReconFile != nil {
		reconPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon.csv", vf.Vendor))
		if err := s.saveUploadedFile(vf.ReconFile, reconPath); err != nil {
			s.log.Errorf("Failed to save %s recon file: %v", vf.Vendor, err)
		} else {
			file, err := os.Open(reconPath)
			if err != nil {
				s.log.Errorf("Failed to open %s recon file: %v", vf.Vendor, err)
			} else {
				defer file.Close()
				switchingData := s.extractReconciliationData(file)
				oldResults := compareReconRRNs(coreData, switchingData)
				
				// Convert to web DTO
				result.ReconResults = s.convertReconResults(oldResults)
				result.ReconMatchCount = s.countMatches(result.ReconResults)
				result.ReconMismatchCount = len(result.ReconResults) - result.ReconMatchCount
				
				// Generate CSV hasil rekonsiliasi
				resultCSVPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_result.csv", vf.Vendor))
				if err := s.writeReconResultCSV(resultCSVPath, oldResults); err != nil {
					s.log.Errorf("Failed to write recon result CSV: %v", err)
				} else {
					s.log.Infof("Generated recon result CSV: %s", resultCSVPath)
				}
			}
		}
	}
	
	// Process Settlement file
	if vf.SettlementFile != nil {
		settlementPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement.txt", vf.Vendor))
		if err := s.saveUploadedFile(vf.SettlementFile, settlementPath); err != nil {
			s.log.Errorf("Failed to save %s settlement file: %v", vf.Vendor, err)
		} else {
			file, err := os.Open(settlementPath)
			if err != nil {
				s.log.Errorf("Failed to open %s settlement file: %v", vf.Vendor, err)
			} else {
				defer file.Close()
				switchingData := s.extractSettlementData(file)
				oldResults := compareSettlementRRNs(coreData, switchingData)
				
				// Convert to web DTO
				result.SettlementResults = s.convertSettlementResults(oldResults)
				result.SettlementMatchCount = s.countMatchesSettlement(result.SettlementResults)
				result.SettlementMismatchCount = len(result.SettlementResults) - result.SettlementMatchCount
				
				// Generate CSV hasil settlement
				resultCSVPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_result.csv", vf.Vendor))
				if err := s.writeSettlementResultCSV(resultCSVPath, oldResults); err != nil {
					s.log.Errorf("Failed to write settlement result CSV: %v", err)
				} else {
					s.log.Infof("Generated settlement result CSV: %s", resultCSVPath)
				}
			}
		}
	}
	
	return result
}

// saveUploadedFile menyimpan file yang diupload ke disk
func (s *ReconciliationService) saveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return err
	}
	
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	
	_, err = io.Copy(out, src)
	return err
}

// extractCoreData mengekstrak data dari core file
func (s *ReconciliationService) extractCoreData(path string) (map[string][]*dto.Data, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1
	
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	
	result := make(map[string][]*dto.Data)
	
	// Skip header
	for i, row := range records {
		if i == 0 {
			continue
		}
		
		if len(row) < 4 {
			continue
		}
		
		supplierName := strings.ToLower(strings.TrimSpace(row[0]))
		rrn := strings.TrimSpace(row[1])
		reff := strings.TrimSpace(row[2])
		status := strings.TrimSpace(row[3])
		
		if rrn == "" {
			continue
		}
		
	data := &dto.Data{
			RRN:    rrn,
			Reff:   reff,
			Status: status,
		}
		
		result[supplierName] = append(result[supplierName], data)
	}
	
	return result, nil
}

// extractReconciliationData mengekstrak data rekonsiliasi dari switching file
func (s *ReconciliationService) extractReconciliationData(file *os.File) map[string]dto.SwitchingReconciliationData {
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1
	
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		s.log.Errorf("Failed to read reconciliation file: %v", err)
		return make(map[string]dto.SwitchingReconciliationData)
	}
	
	result := make(map[string]dto.SwitchingReconciliationData)
	
	for i, row := range records {
		if i == 0 || len(row) < 34 {
			continue
		}
		
		rrn := strings.TrimSpace(row[13])
		if rrn == "" {
			continue
		}
		
		if _, exists := result[rrn]; exists {
			s.log.Warnf("Duplicate RRN found: %s", rrn)
			continue
		}
		
	result[rrn] = dto.SwitchingReconciliationData{
			RRN:            rrn,
			MerchantPAN:    strings.TrimSpace(row[25]),
			Criteria:       strings.TrimSpace(row[21]),
			InvoiceNumber:  strings.TrimSpace(row[31]),
			CreatedDate:    strings.TrimSpace(row[3]),
			CreatedTime:    strings.TrimSpace(row[4]),
			ProcessingCode: strings.TrimSpace(row[32]),
		}
	}
	
	s.log.Infof("Loaded %d records from reconciliation file", len(result))
	return result
}

// extractSettlementData mengekstrak data settlement dari file
func (s *ReconciliationService) extractSettlementData(file *os.File) map[string]dto.SwitchingSettlementData {
	scanner := bufio.NewScanner(file)
	result := make(map[string]dto.SwitchingSettlementData)
	
	inDisputeSection := false
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.Contains(line, "LAPORAN TRANSAKSI DISPUTE") {
			inDisputeSection = true
			continue
		}
		if inDisputeSection && strings.Contains(line, "End of Pages") {
			inDisputeSection = false
			continue
		}
		
		if inDisputeSection {
			continue
		}
		
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "No.") ||
			strings.HasPrefix(line, "---") ||
			strings.Contains(line, "SUB TOTAL") ||
			strings.Contains(line, "TOTAL") {
			continue
		}
		
		if len(line) < 190 || !unicode.IsDigit(rune(line[0])) {
			continue
		}
		
		parsed := parseSettlementDataLine(line)
		if parsed == nil {
			continue
		}
		
		rrn := parsed["Ref_No"]
		
	data := dto.SwitchingSettlementData{
			SwitchingReconciliationData: dto.SwitchingReconciliationData{
				RRN:            rrn,
				MerchantPAN:    parsed["Merchant_PAN"],
				Criteria:       parsed["Merchant_Criteria"],
				InvoiceNumber:  parsed["Trace_No"],
				CreatedDate:    parsed["Tanggal_Trx"],
				CreatedTime:    parsed["Jam_Trx"],
				ProcessingCode: parsed["Trx_Code"],
			},
			ConvenienceFee: parsed["Convenience_Fee"],
			InterchangeFee: parsed["Interchange_Fee"],
		}
		
		result[rrn] = data
	}
	
	return result
}

// parseSettlementDataLine parses settlement fixed-width file
func parseSettlementDataLine(line string) map[string]string {
	line = strings.TrimRight(line, " \t")
	
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}$`)
	interchangeMatches := interchangeRegex.FindStringIndex(line)
	if interchangeMatches == nil {
		return nil
	}
	interchangeFee := line[interchangeMatches[0]:interchangeMatches[1]]
	remaining := line[:interchangeMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	convenienceRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s+[DC]$`)
	convenienceMatches := convenienceRegex.FindStringIndex(remaining)
	if convenienceMatches == nil {
		return nil
	}
	convenienceFee := remaining[convenienceMatches[0]:convenienceMatches[1]]
	remaining = remaining[:convenienceMatches[0]]
	
	parts := strings.Fields(remaining)
	if len(parts) < 12 {
		return nil
	}
	
	return map[string]string{
		"Ref_No":            parts[2],
		"Merchant_PAN":      parts[3],
		"Merchant_Criteria": parts[4],
		"Trace_No":          parts[5],
		"Tanggal_Trx":       parts[6],
		"Jam_Trx":           parts[7],
		"Trx_Code":          parts[8],
		"Convenience_Fee":   convenienceFee,
		"Interchange_Fee":   interchangeFee,
	}
}

// compareReconRRNs membandingkan RRN antara core dan switching
func compareReconRRNs(core []*dto.Data, switching map[string]dto.SwitchingReconciliationData) []dto.ReconciliationSwitchingResult {
	var results []dto.ReconciliationSwitchingResult
	coreMap := make(map[string]*dto.Data)
	
	for _, data := range core {
		coreMap[data.RRN] = data
	}
	
	// RRN exists in both
	for rrn, switchData := range switching {
		if coreData, exists := coreMap[rrn]; exists {
		results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				Reff:             coreData.Reff,
				Status:           coreData.Status,
				MerchantPAN:      switchData.MerchantPAN,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessCode:      switchData.ProcessingCode,
				MatchStatus:      "MATCH",
			})
			delete(coreMap, rrn)
		} else {
		results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				MerchantPAN:      switchData.MerchantPAN,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessCode:      switchData.ProcessingCode,
				MatchStatus:      "ONLY_IN_SWITCHING",
			})
		}
	}
	
	// RRN only in core
	for rrn, coreData := range coreMap {
		results = append(results, dto.ReconciliationSwitchingResult{
			RRN:         rrn,
			Reff:        coreData.Reff,
			Status:      coreData.Status,
			MatchStatus: "ONLY_IN_CORE",
		})
	}
	
	return results
}

// compareSettlementRRNs membandingkan settlement RRNs
func compareSettlementRRNs(core []*dto.Data, switching map[string]dto.SwitchingSettlementData) []dto.SettlementSwitchingResult {
	var results []dto.SettlementSwitchingResult
	coreMap := make(map[string]*dto.Data)
	
	for _, data := range core {
		coreMap[data.RRN] = data
	}
	
	for rrn, switchData := range switching {
		if coreData, exists := coreMap[rrn]; exists {
			results = append(results, dto.SettlementSwitchingResult{
				ReconciliationSwitchingResult: dto.ReconciliationSwitchingResult{
					RRN:              rrn,
					Reff:             coreData.Reff,
					Status:           coreData.Status,
					MerchantPAN:      switchData.MerchantPAN,
					MerchantCriteria: switchData.Criteria,
					InvoiceNumber:    switchData.InvoiceNumber,
					CreatedDate:      switchData.CreatedDate,
					CreatedTime:      switchData.CreatedTime,
					ProcessCode:      switchData.ProcessingCode,
					MatchStatus:      "MATCH",
				},
				InterchangeFee: switchData.InterchangeFee,
				ConvenienceFee: switchData.ConvenienceFee,
			})
			delete(coreMap, rrn)
		} else {
			results = append(results, dto.SettlementSwitchingResult{
				ReconciliationSwitchingResult: dto.ReconciliationSwitchingResult{
					RRN:              rrn,
					MerchantPAN:      switchData.MerchantPAN,
					MerchantCriteria: switchData.Criteria,
					InvoiceNumber:    switchData.InvoiceNumber,
					CreatedDate:      switchData.CreatedDate,
					CreatedTime:      switchData.CreatedTime,
					ProcessCode:      switchData.ProcessingCode,
					MatchStatus:      "ONLY_IN_SWITCHING",
				},
				InterchangeFee: switchData.InterchangeFee,
				ConvenienceFee: switchData.ConvenienceFee,
			})
		}
	}
	
	for rrn, coreData := range coreMap {
		results = append(results, dto.SettlementSwitchingResult{
			ReconciliationSwitchingResult: dto.ReconciliationSwitchingResult{
				RRN:         rrn,
				Reff:        coreData.Reff,
				Status:      coreData.Status,
				MatchStatus: "ONLY_IN_CORE",
			},
		})
	}
	
	return results
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
			MerchantPAN:      r.MerchantPAN,
			MerchantCriteria: r.MerchantCriteria,
			InvoiceNumber:    r.InvoiceNumber,
			CreatedDate:      r.CreatedDate,
			CreatedTime:      r.CreatedTime,
			ProcessCode:      r.ProcessCode,
			MatchStatus:      r.MatchStatus,
			Source:           source,
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
			MerchantPAN:      r.MerchantPAN,
			SettlementAmount: "", // Could be calculated if needed
			InterchangeFee:   r.InterchangeFee,
			ConvenienceFee:   r.ConvenienceFee,
			MatchStatus:      r.MatchStatus,
			Source:           source,
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

// buildDownloadURLs membuat URL download untuk hasil
func (s *ReconciliationService) buildDownloadURLs(jobID string, vendors []dto.VendorFiles) map[string]string {
	urls := make(map[string]string)
	for _, vf := range vendors {
		if vf.ReconFile != nil {
			// URL untuk hasil rekonsiliasi CSV
			urls[vf.Vendor+"_recon_result"] = fmt.Sprintf("/api/download/%s/%s_recon_result.csv", jobID, vf.Vendor)
		}
		if vf.SettlementFile != nil {
			// URL untuk hasil settlement CSV
			urls[vf.Vendor+"_settlement_result"] = fmt.Sprintf("/api/download/%s/%s_settlement_result.csv", jobID, vf.Vendor)
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
		if !entry.IsDir() {
			continue
		}
		
		folderName := entry.Name()
		folderPath := filepath.Join(s.resultsDir, folderName)
		
		// Read files in folder
		files, err := os.ReadDir(folderPath)
		if err != nil {
			s.log.Warnf("Failed to read folder %s: %v", folderName, err)
			continue
		}
		
		var fileList []string
		for _, file := range files {
			if !file.IsDir() {
				fileList = append(fileList, file.Name())
			}
		}
		
		folders = append(folders, map[string]interface{}{
			"name":  folderName,
			"files": fileList,
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
		return nil, fmt.Errorf("result file not found: %s", filename)
	}
	
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open result file: %w", err)
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
		return []map[string]interface{}{}, nil
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
		return []map[string]interface{}{}, nil
	}
	
	// Skip header row
	var results []map[string]interface{}
	
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 10 {
			continue
		}
		
		result := map[string]interface{}{
			"rrn":               row[0],
			"reff":              row[1],
			"status":            row[2],
			"match_status":      row[3],
			"source":            row[4],
			"merchant_pan":      row[5],
			"merchant_criteria": row[6],
			"invoice_number":    row[7],
			"created_date":      row[8],
			"created_time":      row[9],
		}
		
		if len(row) > 10 {
			result["process_code"] = row[10]
		}
		
		results = append(results, result)
	}
	
	return results, nil
}

// parseSettlementResults parses settlement result CSV
func (s *ReconciliationService) parseSettlementResults(records [][]string) ([]map[string]interface{}, error) {
	if len(records) < 2 {
		return []map[string]interface{}{}, nil
	}
	
	// Skip header row
	var results []map[string]interface{}
	
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 6 {
			continue
		}
		
		result := map[string]interface{}{
			"rrn":             row[0],
			"reff":            row[1],
			"status":          row[2],
			"match_status":    row[3],
			"merchant_pan":    row[4],
			"interchange_fee": row[5],
		}
		
		if len(row) > 6 {
			result["convenience_fee"] = row[6]
		}
		
		results = append(results, result)
	}
	
	return results, nil
}

// writeReconResultCSV menulis hasil rekonsiliasi ke CSV
func (s *ReconciliationService) writeReconResultCSV(path string, results []dto.ReconciliationSwitchingResult) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header
	header := []string{
		"RRN", "Reff", "Status", "Match Status", 
		"Merchant PAN", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data rows
	for _, r := range results {
		row := []string{
			r.RRN,
			r.Reff,
			r.Status,
			r.MatchStatus,
			r.MerchantPAN,
			r.MerchantCriteria,
			r.InvoiceNumber,
			r.CreatedDate,
			r.CreatedTime,
			r.ProcessCode,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	
	return nil
}

// writeSettlementResultCSV menulis hasil settlement ke CSV
func (s *ReconciliationService) writeSettlementResultCSV(path string, results []dto.SettlementSwitchingResult) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Write header
	header := []string{
		"RRN", "Reff", "Status", "Match Status",
		"Merchant PAN", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
		"Interchange Fee", "Convenience Fee",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	
	// Write data rows
	for _, r := range results {
		row := []string{
			r.RRN,
			r.Reff,
			r.Status,
			r.MatchStatus,
			r.MerchantPAN,
			r.MerchantCriteria,
			r.InvoiceNumber,
			r.CreatedDate,
			r.CreatedTime,
			r.ProcessCode,
			r.InterchangeFee,
			r.ConvenienceFee,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	
	return nil
}

// extractCSVTags extracts CSV tags from struct
func extractCSVTags(t reflect.Type) []string {
	var headers []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("csv")
		if tag != "" {
			headers = append(headers, tag)
		}
	}
	return headers
}
