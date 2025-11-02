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
	"sort"
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
	jobCounter int // Counter for 4-digit ID
	mu         sync.Mutex
}

// NewReconciliationService creates a new reconciliation service
func NewReconciliationService(log *logrus.Logger, uploadDir, resultsDir string) *ReconciliationService {
	return &ReconciliationService{
		log:        log,
		uploadDir:  uploadDir,
		resultsDir: resultsDir,
		jobCounter: 0, // Will be loaded from existing folders
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
		if err := s.saveUploadedFile(vf.CoreFile, corePath); err != nil {
			s.log.Errorf("Failed to save %s core file: %v", vf.Vendor, err)
			return result
		}
		
		var err error
		coreData, err = s.extractSingleCoreData(corePath)
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
			if err := s.saveUploadedFile(reconFile, reconPath); err != nil {
				s.log.Errorf("Failed to save %s recon file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			// Convert TXT to CSV
			csvPath := filepath.Join(jobDir, fmt.Sprintf("%s_recon_%d.csv", vf.Vendor, idx))
			if err := s.convertReconTxtToCsv(reconPath, csvPath); err != nil {
				s.log.Errorf("Failed to convert %s recon file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			file, err := os.Open(csvPath)
			if err != nil {
				s.log.Errorf("Failed to open %s recon CSV %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			reconData := s.extractReconciliationDataNew(file)
			file.Close()
			
			// Merge data
			for rrn, data := range reconData {
				allReconData[rrn] = data
			}
		}
		
		s.log.Infof("Loaded total %d recon records for vendor %s", len(allReconData), vf.Vendor)
		
		// Compare with CORE
		oldResults := compareReconRRNs(coreData, allReconData)
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
	
	// 3. Process multiple Settlement files
	if len(vf.SettlementFiles) > 0 {
		allSettlementData := make(map[string]dto.SwitchingSettlementData)
		
		for idx, settlementFile := range vf.SettlementFiles {
			settlementPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_%d.txt", vf.Vendor, idx))
			if err := s.saveUploadedFile(settlementFile, settlementPath); err != nil {
				s.log.Errorf("Failed to save %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			// Convert TXT to CSV
			csvPath := filepath.Join(jobDir, fmt.Sprintf("%s_settlement_%d.csv", vf.Vendor, idx))
			if err := s.convertSettlementTxtToCsv(settlementPath, csvPath); err != nil {
				s.log.Errorf("Failed to convert %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			file, err := os.Open(settlementPath)
			if err != nil {
				s.log.Errorf("Failed to open %s settlement file %d: %v", vf.Vendor, idx, err)
				continue
			}
			
			settlementData := s.extractSettlementData(file)
			file.Close()
			
			// Merge data
			for rrn, data := range settlementData {
				allSettlementData[rrn] = data
			}
		}
		
		s.log.Infof("Loaded total %d settlement records for vendor %s", len(allSettlementData), vf.Vendor)
		
		// Compare with CORE
		oldResults := compareSettlementRRNs(coreData, allSettlementData)
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
	
	return result
}

// extractSingleCoreData mengekstrak data dari satu CORE file dengan format baru
func (s *ReconciliationService) extractSingleCoreData(path string) ([]*dto.Data, error) {
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
	
	var result []*dto.Data
	
	// Skip header row (index 0)
	for i, row := range records {
		if i == 0 {
			continue
		}
		
		// Format file CORE baru: kolom 13 adalah RRN
		if len(row) < 14 {
			s.log.Warnf("Row %d has insufficient columns (%d), skipping", i, len(row))
			continue
		}
		
		rrn := strings.TrimSpace(row[13]) // RRN di kolom index 13
		if rrn == "" {
			continue
		}
		
		// Extract other fields based on new CORE format
		data := &dto.Data{
			RRN:          rrn,
			Reff:         strings.TrimSpace(row[10]),  // Reff di kolom 10
			ClientReff:   strings.TrimSpace(row[11]),  // Client Reff di kolom 11
			SupplierReff: strings.TrimSpace(row[12]),  // Supplier Reff di kolom 12
			Status:       strings.TrimSpace(row[1]),   // Status di kolom 1
			CreatedDate:  strings.TrimSpace(row[3]),   // Created Date di kolom 3
			CreatedTime:  strings.TrimSpace(row[4]),   // Created Time di kolom 4
			PaidDate:     strings.TrimSpace(row[5]),   // Paid Date di kolom 5
			PaidTime:     strings.TrimSpace(row[6]),   // Paid Time di kolom 6
		}
		
		if len(row) > 14 {
			data.Vendor = strings.TrimSpace(row[14]) // Supplier Name di kolom 14
		}
		
		result = append(result, data)
	}
	
	return result, nil
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

// extractReconciliationData mengekstrak data rekonsiliasi dari switching file (OLD FORMAT - kept for backward compatibility)
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

// extractReconciliationDataNew mengekstrak data rekonsiliasi dari format baru (pipe-delimited TXT converted to CSV)
func (s *ReconciliationService) extractReconciliationDataNew(file *os.File) map[string]dto.SwitchingReconciliationData {
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1
	
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		s.log.Errorf("Failed to read reconciliation file: %v", err)
		return make(map[string]dto.SwitchingReconciliationData)
	}
	
	result := make(map[string]dto.SwitchingReconciliationData)
	
	// Parse format: DH|Terminal|Trace|MerchantPAN|Date|Time|ProcessCode|Amount|...
	for i, row := range records {
		if i == 0 || len(row) < 18 {
			continue
		}
		
		// Extract RRN from column (perlu mapping sesuai sample file: QR_RECON)
		// Ref_No ada di kolom index yang sesuai format pipe-delimited
		rrn := strings.TrimSpace(row[17]) // Customer PAN sebagai RRN proxy - adjust based on actual format
		
		if rrn == "" {
			continue
		}
		
		if _, exists := result[rrn]; exists {
			s.log.Warnf("Duplicate RRN found: %s", rrn)
			continue
		}
		
		result[rrn] = dto.SwitchingReconciliationData{
			RRN:            rrn,
			MerchantPAN:    strings.TrimSpace(row[3]),
			Criteria:       strings.TrimSpace(row[11]),
			InvoiceNumber:  strings.TrimSpace(row[17]),
			CreatedDate:    strings.TrimSpace(row[4]),
			CreatedTime:    strings.TrimSpace(row[5]),
			ProcessingCode: strings.TrimSpace(row[6]),
		}
	}
	
	s.log.Infof("Loaded %d records from new recon file", len(result))
	return result
}

// convertReconTxtToCsv converts pipe-delimited TXT recon file to CSV
func (s *ReconciliationService) convertReconTxtToCsv(txtPath, csvPath string) error {
	inFile, err := os.Open(txtPath)
	if err != nil {
		return fmt.Errorf("failed to open TXT file: %w", err)
	}
	defer inFile.Close()
	
	outFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer outFile.Close()
	
	writer := csv.NewWriter(outFile)
	defer writer.Flush()
	
	scanner := bufio.NewScanner(inFile)
	lineNum := 0
	
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		
		// Skip header/footer lines
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "LAPORAN") ||
			strings.HasPrefix(line, "No Report") ||
			strings.HasPrefix(line, "---") ||
			strings.Contains(line, "End of Pages") ||
			!strings.HasPrefix(line, "DH|") {
			continue
		}
		
		// Split by pipe delimiter
		fields := strings.Split(line, "|")
		
		// Write to CSV
		if err := writer.Write(fields); err != nil {
			s.log.Warnf("Failed to write line %d: %v", lineNum, err)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning file: %w", err)
	}
	
	s.log.Infof("Converted recon TXT to CSV: %s -> %s", txtPath, csvPath)
	return nil
}

// convertSettlementTxtToCsv converts fixed-width settlement TXT to CSV (removes headers, merges tables)
func (s *ReconciliationService) convertSettlementTxtToCsv(txtPath, csvPath string) error {
	inFile, err := os.Open(txtPath)
	if err != nil {
		return fmt.Errorf("failed to open settlement TXT: %w", err)
	}
	defer inFile.Close()
	
	outFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("failed to create settlement CSV: %w", err)
	}
	defer outFile.Close()
	
	writer := csv.NewWriter(outFile)
	defer writer.Flush()
	
	// Write CSV header
	header := []string{"No", "Trx_Code", "Tanggal_Trx", "Jam_Trx", "Ref_No", "Trace_No", 
		"Terminal_ID", "Merchant_PAN", "Acquirer", "Issuer", "Customer_PAN", "Nominal",
		"Merchant_Category", "Merchant_Criteria", "Response_Code", "Merchant_Name_Location",
		"Convenience_Fee", "Interchange_Fee"}
	writer.Write(header)
	
	scanner := bufio.NewScanner(inFile)
	inDisputeSection := false
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip dispute section
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
		
		// Skip headers and footers
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "No.") ||
			strings.HasPrefix(line, "---") ||
			strings.Contains(line, "SUB TOTAL") ||
			strings.Contains(line, "TOTAL") ||
			strings.Contains(line, "LAPORAN") ||
			strings.Contains(line, "PT ALTO") ||
			strings.Contains(line, "Halaman") {
			continue
		}
		
		// Parse settlement data line (fixed-width format)
		if len(line) < 190 || !unicode.IsDigit(rune(line[0])) {
			continue
		}
		
		parsed := parseSettlementDataLine(line)
		if parsed == nil {
			continue
		}
		
		// Write parsed data as CSV row
		row := []string{
			"", // No (auto-increment bisa ditambahkan kalau perlu)
			parsed["Trx_Code"],
			parsed["Tanggal_Trx"],
			parsed["Jam_Trx"],
			parsed["Ref_No"],
			parsed["Trace_No"],
			"", // Terminal ID
			parsed["Merchant_PAN"],
			"", // Acquirer
			"", // Issuer
			"", // Customer PAN
			"", // Nominal
			"", // Merchant Category
			parsed["Merchant_Criteria"],
			"", // Response Code
			"", // Merchant Name
			parsed["Convenience_Fee"],
			parsed["Interchange_Fee"],
		}
		
		writer.Write(row)
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning settlement file: %w", err)
	}
	
	s.log.Infof("Converted settlement TXT to CSV: %s -> %s", txtPath, csvPath)
	return nil
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
	
	// Extract Interchange Fee (last column, format: -123.00 or 123.00)
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}$`)
	interchangeMatches := interchangeRegex.FindStringIndex(line)
	if interchangeMatches == nil {
		return nil
	}
	interchangeFee := line[interchangeMatches[0]:interchangeMatches[1]]
	remaining := line[:interchangeMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Extract Convenience Fee (second to last column, format: 0.00 C or 0.00 D)
	convenienceRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s+[DC]$`)
	convenienceMatches := convenienceRegex.FindStringIndex(remaining)
	if convenienceMatches == nil {
		return nil
	}
	convenienceFee := remaining[convenienceMatches[0]:convenienceMatches[1]]
	remaining = remaining[:convenienceMatches[0]]
	
	// Split remaining fields by whitespace
	parts := strings.Fields(remaining)
	
	// Expected format (based on actual file):
	// 0:No 1:Trx_Code 2:Tanggal_Trx 3:Jam_Trx 4:Merchant_PAN 5:Trace_No 
	// 6:Terminal_ID 7:Ref_No(RRN) 8:Acquirer 9:Issuer 10:Customer_PAN ...
	
	if len(parts) < 11 {
		return nil
	}
	
	return map[string]string{
		"Ref_No":            parts[7], // RRN is at index 7
		"Merchant_PAN":      parts[4], // Merchant PAN at index 4
		"Merchant_Criteria": parts[5], // Using Trace_No as criteria (or adjust as needed)
		"Trace_No":          parts[5], // Trace No at index 5
		"Tanggal_Trx":       parts[2], // Transaction date at index 2
		"Jam_Trx":           parts[3], // Transaction time at index 3
		"Trx_Code":          parts[1], // Transaction code at index 1
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

// buildDownloadURLs membuat URL download untuk hasil (OLD - kept for backward compatibility)
func (s *ReconciliationService) buildDownloadURLs(jobID string, vendors []dto.VendorFiles) map[string]string {
	urls := make(map[string]string)
	for _, vf := range vendors {
		urls[vf.Vendor+"_recon_result"] = fmt.Sprintf("/api/download/%s/%s_recon_result.csv", jobID, vf.Vendor)
		urls[vf.Vendor+"_settlement_result"] = fmt.Sprintf("/api/download/%s/%s_settlement_result.csv", jobID, vf.Vendor)
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

// ConvertSettlementFile converts settlement TXT file to CSV and returns preview
func (s *ReconciliationService) ConvertSettlementFile(file *multipart.FileHeader) (*dto.SettlementConversionResult, error) {
	// Create converted directory for settlement conversion results
	convertedDir := filepath.Join(s.resultsDir, "converted")
	if err := os.MkdirAll(convertedDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create converted directory: %w", err)
	}
	
	// Create temp directory for temporary upload
	tempDir := filepath.Join(s.uploadDir, "temp")
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	// Generate unique filename for converted CSV
	timestamp := time.Now().Format("20060102_150405")
	baseFilename := strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	csvFilename := fmt.Sprintf("%s_converted_%s.csv", baseFilename, timestamp)
	csvPath := filepath.Join(convertedDir, csvFilename)
	
	// Save uploaded TXT file temporarily
	txtPath := filepath.Join(tempDir, file.Filename)
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()
	
	dst, err := os.Create(txtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return nil, fmt.Errorf("failed to save temp file: %w", err)
	}
	dst.Close()
	
	// Convert TXT to CSV
	if err := s.convertSettlementTxtToCsv(txtPath, csvPath); err != nil {
		os.Remove(txtPath) // Clean up
		return nil, fmt.Errorf("failed to convert settlement file: %w", err)
	}
	
	// Clean up temp TXT file
	os.Remove(txtPath)
	
	// Read CSV to get total records and preview
	csvFile, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted CSV: %w", err)
	}
	defer csvFile.Close()
	
	reader := csv.NewReader(csvFile)
	
	// Read header
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}
	
	// Read all records for count
	allRecords, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV records: %w", err)
	}
	
	totalRecords := len(allRecords)
	
	// Build preview (limit to 100 records to prevent browser overflow)
	previewLimit := 100
	if totalRecords < previewLimit {
		previewLimit = totalRecords
	}
	
	previewRecords := make([]map[string]interface{}, 0, previewLimit)
	for i := 0; i < previewLimit; i++ {
		record := make(map[string]interface{})
		for j, header := range headers {
			if j < len(allRecords[i]) {
				record[header] = allRecords[i][j]
			}
		}
		previewRecords = append(previewRecords, record)
	}
	
	// Build download URL
	downloadURL := fmt.Sprintf("/api/download/converted/%s", csvFilename)
	
	return &dto.SettlementConversionResult{
		Filename:       csvFilename,
		TotalRecords:   totalRecords,
		PreviewRecords: previewRecords,
		DownloadURL:    downloadURL,
	}, nil
}

// GetConvertedFiles returns list of previously converted settlement files
func (s *ReconciliationService) GetConvertedFiles() ([]map[string]interface{}, error) {
	convertedDir := filepath.Join(s.resultsDir, "converted")
	
	// Create directory if not exists
	if err := os.MkdirAll(convertedDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create converted directory: %w", err)
	}
	
	// Read directory contents
	entries, err := os.ReadDir(convertedDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted directory: %w", err)
	}
	
	files := make([]map[string]interface{}, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			s.log.Warnf("Failed to get file info for %s: %v", entry.Name(), err)
			continue
		}
		
		files = append(files, map[string]interface{}{
			"filename":     entry.Name(),
			"size":         info.Size(),
			"modified_at":  info.ModTime(),
			"download_url": fmt.Sprintf("/api/download/converted/%s", entry.Name()),
		})
	}
	
	// Sort by modified time (newest first)
	sort.Slice(files, func(i, j int) bool {
		timeI := files[i]["modified_at"].(time.Time)
		timeJ := files[j]["modified_at"].(time.Time)
		return timeI.After(timeJ)
	})
	
	return files, nil
}
