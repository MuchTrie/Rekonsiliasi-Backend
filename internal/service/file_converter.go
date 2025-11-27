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

// ============================================================================
// FILE CONVERTER - STRUCT & CONSTRUCTOR
// ============================================================================

// FileConverter menangani operasi konversi file dari berbagai format
// Mendukung konversi: TXT → CSV untuk file rekonsiliasi dan settlement
type FileConverter struct {
	log *logrus.Logger
}

// NewFileConverter membuat instance baru dari FileConverter
func NewFileConverter(log *logrus.Logger) *FileConverter {
	return &FileConverter{
		log: log,
	}
}

// ============================================================================
// FUNGSI KONVERSI RECONCILIATION
// ============================================================================

// ConvertReconTxtToCsv mengkonversi file rekonsiliasi TXT (pipe-delimited) ke CSV
// Format input: DH|Terminal|Trace|MerchantPAN|Date|Time|ProcessCode|...
// Format output: CSV dengan delimiter koma
func (fc *FileConverter) ConvertReconTxtToCsv(txtPath, csvPath string) error {
	// Buka file TXT input
	inFile, err := os.Open(txtPath)
	if err != nil {
		return fmt.Errorf("gagal membuka file TXT: %w", err)
	}
	defer inFile.Close()
	
	// Buat file CSV output
	outFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file CSV: %w", err)
	}
	defer outFile.Close()
	
	writer := csv.NewWriter(outFile)
	defer writer.Flush()
	
	scanner := bufio.NewScanner(inFile)
	lineNum := 0
	
	// Loop setiap baris di file TXT
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		
		// Skip header, footer, dan baris kosong
		if strings.TrimSpace(line) == "" ||
			strings.HasPrefix(line, "LAPORAN") ||
			strings.HasPrefix(line, "No Report") ||
			strings.HasPrefix(line, "---") ||
			strings.Contains(line, "End of Pages") ||
			!strings.HasPrefix(line, "DH|") {
			continue
		}
		
		// Split berdasarkan delimiter pipe (|)
		fields := strings.Split(line, "|")
		
		// Tulis ke file CSV
		if err := writer.Write(fields); err != nil {
			fc.log.Warnf("Gagal menulis baris %d: %v", lineNum, err)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error saat scanning file: %w", err)
	}
	
	fc.log.Infof("Konversi rekonsiliasi TXT → CSV berhasil: %s → %s", txtPath, csvPath)
	return nil
}

// ============================================================================
// FUNGSI KONVERSI SETTLEMENT
// ============================================================================

// ConvertSettlementTxtToCsv mengkonversi file settlement TXT ke CSV
// Format settlement: 1 transaksi = 1 baris panjang (500+ karakter)
// Parser menggunakan regex untuk extract fees, lalu split whitespace untuk field lainnya
func (fc *FileConverter) ConvertSettlementTxtToCsv(txtPath, csvPath string) error {
	// Buka file TXT input
	inFile, err := os.Open(txtPath)
	if err != nil {
		return fmt.Errorf("gagal membuka file settlement TXT: %w", err)
	}
	defer inFile.Close()
	
	// Buat file CSV output
	outFile, err := os.Create(csvPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file settlement CSV: %w", err)
	}
	defer outFile.Close()
	
	writer := csv.NewWriter(outFile)
	defer writer.Flush()
	
	// Tulis header CSV
	header := []string{"No", "Trx_Code", "Tanggal_Trx", "Jam_Trx", "Ref_No", "Trace_No", 
		"Terminal_ID", "Merchant_PAN", "Acquirer", "Issuer", "Customer_PAN", "Nominal",
		"Merchant_Category", "Merchant_Criteria", "Response_Code", "Merchant_Name_Location",
		"Convenience_Fee", "Interchange_Fee"}
	writer.Write(header)
	
	// Setup scanner dengan buffer besar untuk baris panjang
	scanner := bufio.NewScanner(inFile)
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)
	
	inDisputeSection := false
	
	// Loop setiap baris di file
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip bagian dispute (tidak perlu diproses)
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
		
		// Skip header, footer, dan baris kosong
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
		
		// Parse baris settlement (1 transaksi = 1 baris panjang)
		// IMPORTANT: Ciptami skip baris < 190 karakter (baris terlalu pendek = invalid/garbage)
		if len(line) >= 190 && unicode.IsDigit(rune(line[0])) {
			// Parse menggunakan single-line parser
			parsed := ParseSettlementDataLineSingleLine(line)
			if parsed != nil {
				// Tulis hasil parsing ke CSV
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
		return fmt.Errorf("error saat scanning file settlement: %w", err)
	}
	
	fc.log.Infof("Konversi settlement TXT → CSV berhasil: %s → %s", txtPath, csvPath)
	return nil
}

// ============================================================================
// FUNGSI PARSER SETTLEMENT - IMPLEMENTASI CIPTAMI
// ============================================================================

// ParseSettlementDataLineSingleLine melakukan parsing baris settlement menggunakan logic Ciptami
// Format: 000001 261000 28/10/25 23:29:55 000116137305 928001 INA-D0417303417 ... 0.00 C -770.00
// Logic diambil langsung dari program Ciptami (switching_reconcile)
func ParseSettlementDataLineSingleLine(line string) map[string]string {
	result := make(map[string]string)
	line = strings.TrimRight(line, "\r\n ")

	// --- STEP 1: Ambil fee dari kanan (karena paling stabil) ---
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}$`)
	interchange := interchangeRegex.FindString(line)
	line = strings.TrimSpace(strings.TrimSuffix(line, interchange))
	result["Interchange_Fee"] = interchange

	convenienceRegex := regexp.MustCompile(`\d+\.\d{2} C$`)
	convenience := convenienceRegex.FindString(line)
	line = strings.TrimSpace(strings.TrimSuffix(line, convenience))
	result["Convenience_Fee"] = convenience

	// --- STEP 2: Pisahkan bagian depan (tanpa fee) ---
	fields := strings.Fields(line)

	// Minimal harus punya 15 field untuk aman
	if len(fields) < 15 {
		return nil
	}

	// Karena Terminal_ID bisa kosong, kita cek panjang Merchant_PAN
	// Pola PAN selalu angka panjang 16–19 digit yang diawali dengan '93600831'
	findPAN := func(start int, fields []string) int {
		for i := start; i < len(fields); i++ {
			if strings.HasPrefix(fields[i], "93600831") && len(fields[i]) >= 16 {
				return i
			}
		}
		return -1
	}

	// Posisi Merchant_PAN kita deteksi otomatis
	panIdx := findPAN(6, fields)
	if panIdx == -1 {
		return nil
	}

	// Terminal_ID bisa kosong → ambil gabungan antara Trace_No dan Merchant_PAN
	result["No"] = fields[0]
	result["Trx_Code"] = fields[1]
	result["Tanggal_Trx"] = fields[2]
	result["Jam_Trx"] = fields[3]
	result["Ref_No"] = fields[4]
	result["Trace_No"] = fields[5]

	if panIdx == 6 {
		result["Terminal_ID"] = ""
	} else {
		result["Terminal_ID"] = fields[6]
	}

	result["Merchant_PAN"] = fields[panIdx]
	result["Acquirer"] = fields[panIdx+1]
	result["Issuer"] = fields[panIdx+2]
	result["Customer_PAN"] = fields[panIdx+3]
	result["Nominal"] = fields[panIdx+4]
	result["Merchant_Category"] = fields[panIdx+5]
	result["Merchant_Criteria"] = fields[panIdx+6]
	result["Response_Code"] = fields[panIdx+7]

	// --- STEP 3: Merchant name & location ---
	merchantStart := panIdx + 8
	if merchantStart < len(fields) {
		result["Merchant_Name_Location"] = strings.Join(fields[merchantStart:], " ")
	}

	return result
}

// ============================================================================
// FUNGSI PARSER SETTLEMENT - MULTI LINE FORMAT (DEPRECATED)
// ============================================================================

// ParseSettlementMultiLine melakukan parsing settlement format multi-line (3 baris per transaksi)
// DEPRECATED: Format lama yang tidak lagi digunakan, disimpan untuk backward compatibility
// Format file settlement saat ini menggunakan single long line per transaksi
//
// Contoh format 3-line:
// Line 1: 000006 266000   28/10/25    23:29:59 1idfhkd57013 903937   INA-A7417383456  9360083133908589201
// Line 2: 93600831    93600915    9360091534900075000        99,000.00 5999              UKE
// Line 3: 00            MATRIX SHOP              TANGERANG    ID          0.00 C         -381.15
func ParseSettlementMultiLine(line1, line2, line3 string) map[string]string {
	// Parse Line 1: No, Trx_Code, Tanggal, Jam, RRN, Trace, Terminal, Merchant PAN
	parts1 := strings.Fields(line1)
	if len(parts1) < 8 {
		return nil
	}
	
	// Parse Line 2: Acquirer, Issuer, Customer PAN, Nominal, Category, Criteria
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
