package service

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
)

// FileConverter handles file conversion operations
type FileConverter struct {
	log *logrus.Logger
}

// NewFileConverter creates a new file converter
func NewFileConverter(log *logrus.Logger) *FileConverter {
	return &FileConverter{
		log: log,
	}
}

// ConvertReconTxtToCsv converts pipe-delimited TXT recon file to CSV
func (fc *FileConverter) ConvertReconTxtToCsv(txtPath, csvPath string) error {
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
			fc.log.Warnf("Failed to write line %d: %v", lineNum, err)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning file: %w", err)
	}
	
	fc.log.Infof("Converted recon TXT to CSV: %s -> %s", txtPath, csvPath)
	return nil
}

// ConvertSettlementTxtToCsv converts fixed-width settlement TXT to CSV
func (fc *FileConverter) ConvertSettlementTxtToCsv(txtPath, csvPath string) error {
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
		
		parsed := ParseSettlementDataLine(line)
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
	
	fc.log.Infof("Converted settlement TXT to CSV: %s -> %s", txtPath, csvPath)
	return nil
}

// ParseSettlementDataLine parses settlement fixed-width file
func ParseSettlementDataLine(line string) map[string]string {
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
