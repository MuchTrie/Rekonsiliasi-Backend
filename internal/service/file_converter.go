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
		if len(line) > 50 && unicode.IsDigit(rune(line[0])) {
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
// FUNGSI PARSER SETTLEMENT - SINGLE LINE FORMAT
// ============================================================================

// ParseSettlementDataLineSingleLine melakukan parsing baris settlement yang panjang (single line)
// Format: 000001 261000 28/10/25 23:29:55 000116137305 928001 INA-D0417303417 ... -770.00
// Algoritma:
//   1. Extract Interchange Fee dari akhir (format: -770.00)
//   2. Extract Convenience Fee sebelumnya (format: 0.00 C)
//   3. Extract Response Code + Merchant Name (format: 00 MERCHANT NAME)
//   4. Split sisanya dengan whitespace untuk dapat 14 field
func ParseSettlementDataLineSingleLine(line string) map[string]string {
	line = strings.TrimRight(line, " \t")
	
	// Step 1: Extract Interchange Fee (kolom terakhir, format: -123.00 atau 123.00)
	interchangeRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s*$`)
	interchangeMatches := interchangeRegex.FindStringIndex(line)
	if interchangeMatches == nil {
		return nil // Tidak valid jika tidak ada Interchange Fee
	}
	interchangeFee := strings.TrimSpace(line[interchangeMatches[0]:interchangeMatches[1]])
	remaining := line[:interchangeMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Step 2: Extract Convenience Fee (kolom kedua dari akhir, format: 0.00 C atau 0.00 D)
	convenienceRegex := regexp.MustCompile(`[+-]?\d{1,3}(?:,\d{3})*\.\d{2}\s+[DC]\s*$`)
	convenienceMatches := convenienceRegex.FindStringIndex(remaining)
	if convenienceMatches == nil {
		return nil // Tidak valid jika tidak ada Convenience Fee
	}
	convenienceFee := strings.TrimSpace(remaining[convenienceMatches[0]:convenienceMatches[1]])
	remaining = remaining[:convenienceMatches[0]]
	
	remaining = strings.TrimRight(remaining, " \t")
	
	// Step 3: Extract Response Code (2 digit) + Merchant Name (sisanya)
	// Format: "00            BUANATELESINO SHOP JKT   JAKARTA SELATID"
	merchantRegex := regexp.MustCompile(`\s+(\d{2})\s+(.+)$`)
	merchantMatches := merchantRegex.FindStringSubmatch(remaining)
	
	merchantName := ""
	responseCode := ""
	if len(merchantMatches) >= 3 {
		responseCode = merchantMatches[1]
		merchantName = strings.TrimSpace(merchantMatches[2])
		remaining = remaining[:len(remaining)-len(merchantMatches[0])]
	}
	
	// Step 4: Split field sisanya berdasarkan whitespace
	parts := strings.Fields(remaining)
	
	// Validasi: Harus ada minimal 14 field
	// Format: No Trx_Code Tanggal_Trx Jam_Trx Ref_No Trace_No Terminal_ID Merchant_PAN Acquirer Issuer Customer_PAN Nominal Merchant_Category Merchant_Criteria
	// 0:No 1:Trx_Code 2:Tanggal_Trx 3:Jam_Trx 4:Ref_No 5:Trace_No 
	// 6:Terminal_ID 7:Merchant_PAN 8:Acquirer 9:Issuer 10:Customer_PAN 11:Nominal 12:Merchant_Category 13:Merchant_Criteria
	
	if len(parts) < 14 {
		return nil // Field tidak lengkap
	}
	
	// Return map dengan key = nama field, value = nilai field
	return map[string]string{
		"No":                     parts[0],   // Nomor urut transaksi
		"Trx_Code":               parts[1],   // Kode transaksi (261000 atau 266000)
		"Tanggal_Trx":            parts[2],   // Tanggal transaksi (DD/MM/YY)
		"Jam_Trx":                parts[3],   // Jam transaksi (HH:MM:SS)
		"Ref_No":                 parts[4],   // RRN (Reference Number)
		"Trace_No":               parts[5],   // Trace Number
		"Terminal_ID":            parts[6],   // Terminal ID
		"Merchant_PAN":           parts[7],   // Merchant PAN
		"Acquirer":               parts[8],   // Acquirer ID
		"Issuer":                 parts[9],   // Issuer ID
		"Customer_PAN":           parts[10],  // Customer PAN
		"Nominal":                parts[11],  // Amount transaksi
		"Merchant_Category":      parts[12],  // Kategori merchant (5999, 7372, dll)
		"Merchant_Criteria":      parts[13],  // Kriteria merchant (UKE, UME, dll)
		"Response_Code":          responseCode,    // Response code (00 = success)
		"Merchant_Name_Location": merchantName,    // Nama dan lokasi merchant
		"Convenience_Fee":        convenienceFee,  // Biaya convenience
		"Interchange_Fee":        interchangeFee,  // Biaya interchange
	}
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
