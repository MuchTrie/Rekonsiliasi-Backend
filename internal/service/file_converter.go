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
	// Increase buffer size for very long lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)
	
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
			strings.Contains(line, "PT JALIN") ||
			strings.Contains(line, "Halaman") {
			continue
		}
		
		// Parse settlement data line - each transaction is ONE long line
		if len(line) > 50 && unicode.IsDigit(rune(line[0])) {
			parsed := ParseSettlementDataLineSingleLine(line)
			if parsed != nil {
				// Write parsed data as CSV row
				row := []string{
					parsed["No"],
					parsed["Trx_Code"],
					parsed["Tanggal_Trx"],
					parsed["Jam_Trx"],
					parsed["Ref_No"],
					parsed["Trace_No"],
					parsed["Terminal_ID"],
					parsed["Merchant_PAN"],
					parsed["Acquirer"],
					parsed["Issuer"],
					parsed["Customer_PAN"],
					parsed["Nominal"],
					parsed["Merchant_Category"],
					parsed["Merchant_Criteria"],
					parsed["Response_Code"],
					parsed["Merchant_Name_Location"],
					parsed["Convenience_Fee"],
					parsed["Interchange_Fee"],
				}
				writer.Write(row)
			}
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning settlement file: %w", err)
	}
	
	fc.log.Infof("Converted settlement TXT to CSV: %s -> %s", txtPath, csvPath)
	return nil
}

// ParseSettlementDataLineSingleLine parses settlement single long line
// Format: 000001 261000   28/10/25    23:29:55 000116137305 928001   INA-D0417303417  9360083135739049578 93600831    93600014    9360001410098409682       200,000.00 5999              UKE               00            BUANATELESINO SHOP JKT   JAKARTA SELATID          0.00 C         -770.00
func ParseSettlementDataLineSingleLine(line string) map[string]string {
	line = strings.TrimRight(line, " \t")
	
	// Extract Interchange Fee (last column, format: -123.00 or 123.00)
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s*$`)
	interchangeMatches := interchangeRegex.FindStringIndex(line)
	if interchangeMatches == nil {
		return nil
	}
	interchangeFee := strings.TrimSpace(line[interchangeMatches[0]:interchangeMatches[1]])
	remaining := line[:interchangeMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Extract Convenience Fee (second to last column, format: 0.00 C or 0.00 D)
	convenienceRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s+[DC]\s*$`)
	convenienceMatches := convenienceRegex.FindStringIndex(remaining)
	if convenienceMatches == nil {
		return nil
	}
	convenienceFee := strings.TrimSpace(remaining[convenienceMatches[0]:convenienceMatches[1]])
	remaining = remaining[:convenienceMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Extract merchant name and location (everything before response code)
	// Response code is typically 2 digits followed by multiple spaces
	merchantRegex := regexp.MustCompile(`\s+(\d{2})\s+(.+)$`)
	merchantMatches := merchantRegex.FindStringSubmatch(remaining)
	
	merchantName := ""
	responseCode := ""
	if len(merchantMatches) >= 3 {
		responseCode = merchantMatches[1]
		merchantName = strings.TrimSpace(merchantMatches[2])
		remaining = remaining[:len(remaining)-len(merchantMatches[0])]
	}
	
	// Split remaining fields by whitespace
	parts := strings.Fields(remaining)
	
	// Expected format after removing fees, merchant name, and response code:
	// 0:No 1:Trx_Code 2:Tanggal_Trx 3:Jam_Trx 4:Ref_No 5:Trace_No 
	// 6:Terminal_ID 7:Merchant_PAN 8:Acquirer 9:Issuer 10:Customer_PAN 11:Nominal 12:Merchant_Category 13:Merchant_Criteria
	
	if len(parts) < 14 {
		return nil
	}
	
	return map[string]string{
		"No":                     parts[0],
		"Trx_Code":               parts[1],
		"Tanggal_Trx":            parts[2],
		"Jam_Trx":                parts[3],
		"Ref_No":                 parts[4],  // RRN
		"Trace_No":               parts[5],
		"Terminal_ID":            parts[6],
		"Merchant_PAN":           parts[7],
		"Acquirer":               parts[8],
		"Issuer":                 parts[9],
		"Customer_PAN":           parts[10],
		"Nominal":                parts[11], // Amount
		"Merchant_Category":      parts[12],
		"Merchant_Criteria":      parts[13],
		"Response_Code":          responseCode,
		"Merchant_Name_Location": merchantName,
		"Convenience_Fee":        convenienceFee,
		"Interchange_Fee":        interchangeFee,
	}
}

// ParseSettlementMultiLine parses settlement multi-line record (3 lines per transaction) - DEPRECATED
// Kept for backward compatibility, but current file format uses single long lines
// Line 1: 000006 266000   28/10/25    23:29:59 1idfhkd57013 903937   INA-A7417383456  9360083133908589201
// Line 2: 93600831    93600915    9360091534900075000        99,000.00 5999              UKE
// Line 3: 00            MATRIX SHOP              TANGERANG    ID          0.00 C         -381.15
func ParseSettlementMultiLine(line1, line2, line3 string) map[string]string {
	// Parse Line 1
	parts1 := strings.Fields(line1)
	if len(parts1) < 8 {
		return nil
	}
	
	// Parse Line 2 to get Nominal, Merchant Category, Merchant Criteria
	parts2 := strings.Fields(line2)
	if len(parts2) < 6 {
		return nil
	}
	
	// Parse Line 3 to get Response Code, Merchant Name, Convenience Fee, Interchange Fee
	line3 = strings.TrimRight(line3, " \t")
	
	// Extract Interchange Fee (last column, format: -123.00 or 123.00)
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}$`)
	interchangeMatches := interchangeRegex.FindStringIndex(line3)
	if interchangeMatches == nil {
		return nil
	}
	interchangeFee := line3[interchangeMatches[0]:interchangeMatches[1]]
	remaining := line3[:interchangeMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Extract Convenience Fee (second to last column, format: 0.00 C or 0.00 D)
	convenienceRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s+[DC]$`)
	convenienceMatches := convenienceRegex.FindStringIndex(remaining)
	if convenienceMatches == nil {
		return nil
	}
	convenienceFee := remaining[convenienceMatches[0]:convenienceMatches[1]]
	remaining = remaining[:convenienceMatches[0]]
	
	// Parse remaining fields in line 3
	parts3 := strings.Fields(remaining)
	
	merchantName := ""
	responseCode := ""
	if len(parts3) >= 2 {
		responseCode = parts3[0]
		merchantName = strings.Join(parts3[1:], " ")
	}
	
	return map[string]string{
		"No":                     parts1[0],
		"Trx_Code":               parts1[1],
		"Tanggal_Trx":            parts1[2],
		"Jam_Trx":                parts1[3],
		"Ref_No":                 parts1[4], // RRN
		"Trace_No":               parts1[5],
		"Terminal_ID":            parts1[6],
		"Merchant_PAN":           parts1[7],
		"Acquirer":               parts2[0],
		"Issuer":                 parts2[1],
		"Customer_PAN":           parts2[2],
		"Nominal":                parts2[3], // Amount
		"Merchant_Category":      parts2[4],
		"Merchant_Criteria":      parts2[5],
		"Response_Code":          responseCode,
		"Merchant_Name_Location": merchantName,
		"Convenience_Fee":        convenienceFee,
		"Interchange_Fee":        interchangeFee,
	}
}

// ParseSettlementDataLine parses settlement fixed-width file (DEPRECATED - use ParseSettlementMultiLine instead)
// Kept for backward compatibility
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
	
	// Expected format (based on actual file after removing Convenience_Fee and Interchange_Fee):
	// 0:No 1:Trx_Code 2:Tanggal_Trx 3:Jam_Trx 4:Ref_No(RRN) 5:Trace_No 
	// 6:Terminal_ID 7:Merchant_PAN 8:Acquirer 9:Issuer 10:Customer_PAN 11:Nominal 12:Merchant_Category 13:Merchant_Criteria ...
	
	if len(parts) < 14 {
		return nil
	}
	
	return map[string]string{
		"Ref_No":            parts[4],  // RRN is at index 4 ✅ DIPERBAIKI!
		"Merchant_PAN":      parts[7],  // Merchant PAN at index 7
		"Merchant_Criteria": parts[13], // Merchant Criteria at index 13
		"Trace_No":          parts[5],  // Trace No at index 5
		"Tanggal_Trx":       parts[2],  // Transaction date at index 2
		"Jam_Trx":           parts[3],  // Transaction time at index 3
		"Trx_Code":          parts[1],  // Transaction code at index 1
		"Nominal":           parts[11], // Nominal/Amount at index 11
		"Convenience_Fee":   convenienceFee,
		"Interchange_Fee":   interchangeFee,
	}
}
