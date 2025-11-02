package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
)

// saveUploadedFile menyimpan file yang diupload ke disk
func saveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()
	
	_, err = io.Copy(out, src)
	return err
}

// WriteReconResultCSV menulis hasil rekonsiliasi ke CSV
func WriteReconResultCSV(path string, results []dto.ReconciliationSwitchingResult) error {
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
		return fmt.Errorf("failed to write header: %w", err)
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
			r.ProcessingCode,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}
	
	return nil
}

// WriteSettlementResultCSV menulis hasil settlement ke CSV
func WriteSettlementResultCSV(path string, results []dto.SettlementSwitchingResult) error {
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
		return fmt.Errorf("failed to write header: %w", err)
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
			r.ProcessingCode,
			r.InterchangeFee,
			r.ConvenienceFee,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}
	
	return nil
}
