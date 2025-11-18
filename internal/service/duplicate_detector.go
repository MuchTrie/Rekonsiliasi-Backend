package service

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ============================================================================
// DUPLICATE DETECTOR - STRUCT & CONSTRUCTOR
// ============================================================================

// DuplicateDetector handles duplicate RRN detection across all files
type DuplicateDetector struct {
	log *logrus.Logger
}

// NewDuplicateDetector creates a new instance of DuplicateDetector
func NewDuplicateDetector(log *logrus.Logger) *DuplicateDetector {
	return &DuplicateDetector{
		log: log,
	}
}

// ============================================================================
// CORE DUPLICATE DETECTION
// ============================================================================

// DetectCoreDuplicates detects duplicate RRNs in CORE files
// Returns a slice of DuplicateGroup for each RRN that appears more than once
func (dd *DuplicateDetector) DetectCoreDuplicates(corePath string) ([]dto.DuplicateGroup, error) {
	dd.log.Infof("Starting duplicate detection for CORE file: %s", corePath)

	file, err := os.Open(corePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CORE file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CORE CSV: %w", err)
	}

	// Map untuk tracking RRN: RRN -> []DuplicateRecord
	rrnMap := make(map[string][]dto.DuplicateRecord)

	// Loop through records (skip header)
	for i, row := range records {
		if i == 0 {
			continue // Skip header
		}

		// Validasi jumlah kolom
		if len(row) < 16 {
			continue
		}

		rrn := strings.TrimSpace(row[13]) // RRN di kolom 13
		if rrn == "" {
			continue
		}

		amount := AmountConverter(row[15], dd.log) // Amount di kolom 15

		// Create duplicate record
		record := dto.DuplicateRecord{
			RRN:         rrn,
			Amount:      amount,
			LineNumber:  i + 1, // Line number (1-based)
			Source:      "CORE",
			FileName:    corePath,
			Vendor:      strings.TrimSpace(row[14]), // Supplier Name
			CreatedDate: strings.TrimSpace(row[3]),
			CreatedTime: strings.TrimSpace(row[4]),
		}

		// Tambahkan ke map
		rrnMap[rrn] = append(rrnMap[rrn], record)
	}

	// Filter hanya RRN yang muncul lebih dari 1x
	var duplicates []dto.DuplicateGroup
	for rrn, records := range rrnMap {
		if len(records) > 1 {
			// Calculate total amount
			var totalAmount float64
			for _, rec := range records {
				totalAmount += rec.Amount
			}

			group := dto.DuplicateGroup{
				RRN:             rrn,
				OccurrenceCount: len(records),
				Records:         records,
				TotalAmount:     totalAmount,
			}
			duplicates = append(duplicates, group)
		}
	}

	dd.log.Infof("Found %d duplicate RRNs in CORE file (%d total duplicate records)",
		len(duplicates), countTotalRecords(duplicates))

	return duplicates, nil
}

// ============================================================================
// RECONCILIATION DUPLICATE DETECTION
// ============================================================================

// DetectReconDuplicates detects duplicate RRNs in Reconciliation files
func (dd *DuplicateDetector) DetectReconDuplicates(reconPath, vendor string) ([]dto.DuplicateGroup, error) {
	dd.log.Infof("Starting duplicate detection for RECON file: %s (vendor: %s)", reconPath, vendor)

	file, err := os.Open(reconPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open RECON file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read RECON CSV: %w", err)
	}

	// Map untuk tracking RRN
	rrnMap := make(map[string][]dto.DuplicateRecord)

	// Loop through records (NO header in converted files)
	for i, row := range records {
		// Validasi jumlah kolom
		if len(row) < 17 {
			continue
		}

		rrn := strings.TrimSpace(row[2]) // Trace Number (RRN) di kolom 2
		if rrn == "" {
			continue
		}

		amount := AmountConverter(row[7], dd.log) // Amount di kolom 7

		record := dto.DuplicateRecord{
			RRN:         rrn,
			Amount:      amount,
			LineNumber:  i + 1,
			Source:      "SWITCHING_RECON",
			FileName:    reconPath,
			Vendor:      vendor,
			CreatedDate: strings.TrimSpace(row[8]), // Created Date di kolom 8
			CreatedTime: strings.TrimSpace(row[9]), // Created Time di kolom 9
		}

		rrnMap[rrn] = append(rrnMap[rrn], record)
	}

	// Filter duplicates
	var duplicates []dto.DuplicateGroup
	for rrn, records := range rrnMap {
		if len(records) > 1 {
			var totalAmount float64
			for _, rec := range records {
				totalAmount += rec.Amount
			}

			group := dto.DuplicateGroup{
				RRN:             rrn,
				OccurrenceCount: len(records),
				Records:         records,
				TotalAmount:     totalAmount,
			}
			duplicates = append(duplicates, group)
		}
	}

	dd.log.Infof("Found %d duplicate RRNs in RECON file (%d total duplicate records)",
		len(duplicates), countTotalRecords(duplicates))

	return duplicates, nil
}

// ============================================================================
// SETTLEMENT DUPLICATE DETECTION
// ============================================================================

// DetectSettlementDuplicates detects duplicate RRNs in Settlement files
func (dd *DuplicateDetector) DetectSettlementDuplicates(settlePath, vendor string) ([]dto.DuplicateGroup, error) {
	dd.log.Infof("Starting duplicate detection for SETTLEMENT file: %s (vendor: %s)", settlePath, vendor)

	file, err := os.Open(settlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SETTLEMENT file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read SETTLEMENT CSV: %w", err)
	}

	// Map untuk tracking RRN
	rrnMap := make(map[string][]dto.DuplicateRecord)

	// Loop through records (NO header)
	for i, row := range records {
		// Validasi jumlah kolom (settlement memiliki lebih banyak kolom)
		if len(row) < 19 {
			continue
		}

		rrn := strings.TrimSpace(row[4]) // Ref No (RRN) di kolom 4
		if rrn == "" {
			continue
		}

		amount := AmountConverter(row[11], dd.log) // Nominal di kolom 11

		record := dto.DuplicateRecord{
			RRN:         rrn,
			Amount:      amount,
			LineNumber:  i + 1,
			Source:      "SWITCHING_SETTLEMENT",
			FileName:    settlePath,
			Vendor:      vendor,
			CreatedDate: strings.TrimSpace(row[2]), // Tanggal Trx
			CreatedTime: strings.TrimSpace(row[3]), // Jam Trx
		}

		rrnMap[rrn] = append(rrnMap[rrn], record)
	}

	// Filter duplicates
	var duplicates []dto.DuplicateGroup
	for rrn, records := range rrnMap {
		if len(records) > 1 {
			var totalAmount float64
			for _, rec := range records {
				totalAmount += rec.Amount
			}

			group := dto.DuplicateGroup{
				RRN:             rrn,
				OccurrenceCount: len(records),
				Records:         records,
				TotalAmount:     totalAmount,
			}
			duplicates = append(duplicates, group)
		}
	}

	dd.log.Infof("Found %d duplicate RRNs in SETTLEMENT file (%d total duplicate records)",
		len(duplicates), countTotalRecords(duplicates))

	return duplicates, nil
}

// ============================================================================
// GENERATE COMPLETE REPORT
// ============================================================================

// GenerateDuplicateReport generates a complete duplicate report for all files
func (dd *DuplicateDetector) GenerateDuplicateReport(
	jobID string,
	corePath string,
	reconPaths map[string]string,    // vendor -> file path
	settlePaths map[string]string,   // vendor -> file path
) (*dto.DuplicateReport, error) {

	dd.log.Infof("Generating complete duplicate report for job: %s", jobID)

	report := &dto.DuplicateReport{
		JobID:            jobID,
		CoreDuplicates:   []dto.DuplicateGroup{},
		ReconDuplicates:  []dto.DuplicateGroup{},
		SettleDuplicates: []dto.DuplicateGroup{},
		GeneratedAt:      time.Now().Format("2006-01-02 15:04:05"),
	}

	// 1. Detect CORE duplicates
	if corePath != "" {
		coreDups, err := dd.DetectCoreDuplicates(corePath)
		if err != nil {
			dd.log.Warnf("Failed to detect CORE duplicates: %v", err)
		} else {
			report.CoreDuplicates = coreDups
		}
	}

	// 2. Detect RECON duplicates for each vendor
	for vendor, path := range reconPaths {
		if path == "" {
			continue
		}
		reconDups, err := dd.DetectReconDuplicates(path, vendor)
		if err != nil {
			dd.log.Warnf("Failed to detect RECON duplicates for %s: %v", vendor, err)
		} else {
			report.ReconDuplicates = append(report.ReconDuplicates, reconDups...)
		}
	}

	// 3. Detect SETTLEMENT duplicates for each vendor
	for vendor, path := range settlePaths {
		if path == "" {
			continue
		}
		settleDups, err := dd.DetectSettlementDuplicates(path, vendor)
		if err != nil {
			dd.log.Warnf("Failed to detect SETTLEMENT duplicates for %s: %v", vendor, err)
		} else {
			report.SettleDuplicates = append(report.SettleDuplicates, settleDups...)
		}
	}

	// Calculate totals
	report.TotalDuplicates = len(report.CoreDuplicates) + len(report.ReconDuplicates) + len(report.SettleDuplicates)
	report.TotalRecords = countTotalRecords(report.CoreDuplicates) +
		countTotalRecords(report.ReconDuplicates) +
		countTotalRecords(report.SettleDuplicates)

	dd.log.Infof("Duplicate report generated: %d unique duplicate RRNs, %d total duplicate records",
		report.TotalDuplicates, report.TotalRecords)

	return report, nil
}

// ============================================================================
// EXPORT TO CSV
// ============================================================================

// ExportDuplicatesToCSV exports duplicate report to CSV file
func (dd *DuplicateDetector) ExportDuplicatesToCSV(report *dto.DuplicateReport, outputPath string) error {
	dd.log.Infof("Exporting duplicate report to CSV: %s", outputPath)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"RRN",
		"Source",
		"Vendor",
		"Occurrence Count",
		"Line Number",
		"Amount",
		"Total Amount",
		"Created Date",
		"Created Time",
		"File Name",
	}
	writer.Write(header)

	// Helper function to write duplicate groups
	writeDuplicates := func(groups []dto.DuplicateGroup) {
		for _, group := range groups {
			for _, record := range group.Records {
				row := []string{
					record.RRN,
					record.Source,
					record.Vendor,
					fmt.Sprintf("%d", group.OccurrenceCount),
					fmt.Sprintf("%d", record.LineNumber),
					fmt.Sprintf("%.2f", record.Amount),
					fmt.Sprintf("%.2f", group.TotalAmount),
					record.CreatedDate,
					record.CreatedTime,
					record.FileName,
				}
				writer.Write(row)
			}
		}
	}

	// Write all duplicates
	writeDuplicates(report.CoreDuplicates)
	writeDuplicates(report.ReconDuplicates)
	writeDuplicates(report.SettleDuplicates)

	dd.log.Infof("Successfully exported %d duplicate records to CSV", report.TotalRecords)
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// countTotalRecords counts total duplicate records across all groups
func countTotalRecords(groups []dto.DuplicateGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Records)
	}
	return total
}
