package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ============================================================================
// FUNGSI UTILITY - HELPER FUNCTIONS
// ============================================================================

// AmountConverter mengkonversi string amount menjadi float64
// Menangani format seperti "10,000.00" atau "10000.00"
// Mengembalikan 0.0 jika parsing gagal
func AmountConverter(amount string, log *logrus.Logger) float64 {
	// Hapus koma dan whitespace
	cleanStr := strings.TrimSpace(strings.ReplaceAll(amount, ",", ""))
	
	// Parse ke float64
	f, err := strconv.ParseFloat(cleanStr, 64)
	if err != nil {
		if log != nil {
			log.Warnf("Gagal parse amount '%s': %v, menggunakan 0.00", amount, err)
		}
		return 0.0
	}
	
	return f
}

// saveUploadedFile menyimpan file yang diupload ke disk
func saveUploadedFile(file *multipart.FileHeader, dst string) error {
	// Buka file upload
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("gagal membuka file: %w", err)
	}
	defer src.Close()
	
	// Pastikan direktori ada
	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return fmt.Errorf("gagal membuat direktori: %w", err)
	}
	
	// Buat file tujuan
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("gagal membuat file: %w", err)
	}
	defer out.Close()
	
	// Copy isi file
	_, err = io.Copy(out, src)
	return err
}

// ============================================================================
// FUNGSI EXPORT - CSV WRITER FUNCTIONS
// ============================================================================

// WriteReconResultCSV menulis hasil rekonsiliasi ke file CSV
func WriteReconResultCSV(path string, results []dto.ReconciliationSwitchingResult) error {
	// Buat file CSV
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("gagal membuat file CSV: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Tulis header CSV
	header := []string{
		"No", "RRN", "Reff", "Status", "Match Status", 
		"Merchant PAN", "Merchant Name", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("gagal menulis header: %w", err)
	}
	
	// Tulis data row by row
	for i, r := range results {
		row := []string{
			fmt.Sprintf("%d", i+1), // Nomor urut
			r.RRN,
			r.Reff,
			r.Status,
			r.MatchStatus,
			r.MerchantPAN,
			r.MerchantName,
			r.MerchantCriteria,
			r.InvoiceNumber,
			r.CreatedDate,
			r.CreatedTime,
			r.ProcessingCode,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("gagal menulis row: %w", err)
		}
	}
	
	return nil
}

// WriteSettlementResultCSV menulis hasil settlement ke file CSV
func WriteSettlementResultCSV(path string, results []dto.SettlementSwitchingResult) error {
	// Buat file CSV
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("gagal membuat file CSV: %w", err)
	}
	defer file.Close()
	
	writer := csv.NewWriter(file)
	defer writer.Flush()
	
	// Tulis header CSV
	header := []string{
		"No", "RRN", "Amount", "Reff", "Status", "Match Status",
		"Merchant PAN", "Merchant Name", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
		"Interchange Fee", "Convenience Fee",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("gagal menulis header: %w", err)
	}
	
	// Tulis data row by row
	for i, r := range results {
		row := []string{
			fmt.Sprintf("%d", i+1), // Nomor urut
			r.RRN,
			fmt.Sprintf("%.2f", r.Amount), // Format amount dengan 2 desimal
			r.Reff,
			r.Status,
			r.MatchStatus,
			r.MerchantPAN,
			r.MerchantName,
			r.MerchantCriteria,
			r.InvoiceNumber,
			r.CreatedDate,
			r.CreatedTime,
			r.ProcessingCode,
			r.InterchangeFee,
			r.ConvenienceFee,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("gagal menulis row: %w", err)
		}
	}
	
	return nil
}
