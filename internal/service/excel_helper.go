package service

import (
	"fmt"
	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/xuri/excelize/v2"
)

// WriteReconResultExcel menulis hasil rekonsiliasi ke file Excel dengan sheets terpisah
// Sheet 1: Data Lebih di Switching (ONLY_IN_SWITCHING)
// Sheet 2: Data Lebih di Core (ONLY_IN_CORE)
// Sheet 3: Data Match (MATCH)
func WriteReconResultExcel(path string, results []dto.ReconciliationSwitchingResult) error {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing Excel file: %v\n", err)
		}
	}()

	// Pisahkan data berdasarkan match status
	var onlyInSwitching []dto.ReconciliationSwitchingResult
	var onlyInCore []dto.ReconciliationSwitchingResult
	var matched []dto.ReconciliationSwitchingResult

	for _, r := range results {
		switch r.MatchStatus {
		case "ONLY_IN_SWITCHING":
			onlyInSwitching = append(onlyInSwitching, r)
		case "ONLY_IN_CORE":
			onlyInCore = append(onlyInCore, r)
		case "MATCH":
			matched = append(matched, r)
		}
	}

	// Header untuk rekonsiliasi
	header := []string{
		"No", "RRN", "Reff", "Status", "Match Status",
		"Merchant PAN", "Merchant Name", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
	}

	// Sheet 1: Data Lebih di Switching
	sheetSwitching := "Lebih di Switching"
	index, err := f.NewSheet(sheetSwitching)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet switching: %w", err)
	}
	f.SetActiveSheet(index)

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetSwitching, cell, h)
	}

	// Write data Lebih di Switching
	for idx, r := range onlyInSwitching {
		rowNum := idx + 2
		f.SetCellValue(sheetSwitching, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("C%d", rowNum), r.Reff)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("D%d", rowNum), r.Status)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("E%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("F%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("G%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("H%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("I%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("J%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("K%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("L%d", rowNum), r.ProcessingCode)
	}

	// Sheet 2: Data Lebih di Core
	sheetCore := "Lebih di Core"
	_, err = f.NewSheet(sheetCore)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet core: %w", err)
	}

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetCore, cell, h)
	}

	// Write data Lebih di Core
	for idx, r := range onlyInCore {
		rowNum := idx + 2
		f.SetCellValue(sheetCore, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetCore, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetCore, fmt.Sprintf("C%d", rowNum), r.Reff)
		f.SetCellValue(sheetCore, fmt.Sprintf("D%d", rowNum), r.Status)
		f.SetCellValue(sheetCore, fmt.Sprintf("E%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetCore, fmt.Sprintf("F%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetCore, fmt.Sprintf("G%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetCore, fmt.Sprintf("H%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetCore, fmt.Sprintf("I%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetCore, fmt.Sprintf("J%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetCore, fmt.Sprintf("K%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetCore, fmt.Sprintf("L%d", rowNum), r.ProcessingCode)
	}

	// Sheet 3: Data Match
	sheetMatch := "Data Match"
	_, err = f.NewSheet(sheetMatch)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet match: %w", err)
	}

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetMatch, cell, h)
	}

	// Write data Match
	for idx, r := range matched {
		rowNum := idx + 2
		f.SetCellValue(sheetMatch, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetMatch, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetMatch, fmt.Sprintf("C%d", rowNum), r.Reff)
		f.SetCellValue(sheetMatch, fmt.Sprintf("D%d", rowNum), r.Status)
		f.SetCellValue(sheetMatch, fmt.Sprintf("E%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetMatch, fmt.Sprintf("F%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetMatch, fmt.Sprintf("G%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetMatch, fmt.Sprintf("H%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetMatch, fmt.Sprintf("I%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetMatch, fmt.Sprintf("J%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetMatch, fmt.Sprintf("K%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetMatch, fmt.Sprintf("L%d", rowNum), r.ProcessingCode)
	}

	// Hapus Sheet1 default
	f.DeleteSheet("Sheet1")

	// Save file
	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("gagal menyimpan file Excel: %w", err)
	}

	return nil
}

// WriteSettlementResultExcel menulis hasil settlement ke file Excel dengan sheets terpisah
// Sheet 1: Data Lebih di Switching (ONLY_IN_SWITCHING)
// Sheet 2: Data Lebih di Core (ONLY_IN_CORE)
// Sheet 3: Data Match (MATCH)
func WriteSettlementResultExcel(path string, results []dto.SettlementSwitchingResult) error {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing Excel file: %v\n", err)
		}
	}()

	// Pisahkan data berdasarkan match status
	var onlyInSwitching []dto.SettlementSwitchingResult
	var onlyInCore []dto.SettlementSwitchingResult
	var matched []dto.SettlementSwitchingResult

	for _, r := range results {
		switch r.MatchStatus {
		case "ONLY_IN_SWITCHING":
			onlyInSwitching = append(onlyInSwitching, r)
		case "ONLY_IN_CORE":
			onlyInCore = append(onlyInCore, r)
		case "MATCH":
			matched = append(matched, r)
		}
	}

	// Header untuk settlement
	header := []string{
		"No", "RRN", "Amount", "Reff", "Status", "Match Status",
		"Merchant PAN", "Merchant Name", "Merchant Criteria", "Invoice Number",
		"Created Date", "Created Time", "Processing Code",
		"Interchange Fee", "Convenience Fee",
	}

	// Sheet 1: Data Lebih di Switching
	sheetSwitching := "Lebih di Switching"
	index, err := f.NewSheet(sheetSwitching)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet switching: %w", err)
	}
	f.SetActiveSheet(index)

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetSwitching, cell, h)
	}

	// Write data Lebih di Switching
	for idx, r := range onlyInSwitching {
		rowNum := idx + 2
		f.SetCellValue(sheetSwitching, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("C%d", rowNum), r.Amount)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("D%d", rowNum), r.Reff)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("E%d", rowNum), r.Status)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("F%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("G%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("H%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("I%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("J%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("K%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("L%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("M%d", rowNum), r.ProcessingCode)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("N%d", rowNum), r.InterchangeFee)
		f.SetCellValue(sheetSwitching, fmt.Sprintf("O%d", rowNum), r.ConvenienceFee)
	}

	// Sheet 2: Data Lebih di Core
	sheetCore := "Lebih di Core"
	_, err = f.NewSheet(sheetCore)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet core: %w", err)
	}

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetCore, cell, h)
	}

	// Write data Lebih di Core
	for idx, r := range onlyInCore {
		rowNum := idx + 2
		f.SetCellValue(sheetCore, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetCore, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetCore, fmt.Sprintf("C%d", rowNum), r.Amount)
		f.SetCellValue(sheetCore, fmt.Sprintf("D%d", rowNum), r.Reff)
		f.SetCellValue(sheetCore, fmt.Sprintf("E%d", rowNum), r.Status)
		f.SetCellValue(sheetCore, fmt.Sprintf("F%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetCore, fmt.Sprintf("G%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetCore, fmt.Sprintf("H%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetCore, fmt.Sprintf("I%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetCore, fmt.Sprintf("J%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetCore, fmt.Sprintf("K%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetCore, fmt.Sprintf("L%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetCore, fmt.Sprintf("M%d", rowNum), r.ProcessingCode)
		f.SetCellValue(sheetCore, fmt.Sprintf("N%d", rowNum), r.InterchangeFee)
		f.SetCellValue(sheetCore, fmt.Sprintf("O%d", rowNum), r.ConvenienceFee)
	}

	// Sheet 3: Data Match
	sheetMatch := "Data Match"
	_, err = f.NewSheet(sheetMatch)
	if err != nil {
		return fmt.Errorf("gagal membuat sheet match: %w", err)
	}

	// Write header
	for i, h := range header {
		cell := fmt.Sprintf("%s1", string(rune('A'+i)))
		f.SetCellValue(sheetMatch, cell, h)
	}

	// Write data Match
	for idx, r := range matched {
		rowNum := idx + 2
		f.SetCellValue(sheetMatch, fmt.Sprintf("A%d", rowNum), idx+1)
		f.SetCellValue(sheetMatch, fmt.Sprintf("B%d", rowNum), r.RRN)
		f.SetCellValue(sheetMatch, fmt.Sprintf("C%d", rowNum), r.Amount)
		f.SetCellValue(sheetMatch, fmt.Sprintf("D%d", rowNum), r.Reff)
		f.SetCellValue(sheetMatch, fmt.Sprintf("E%d", rowNum), r.Status)
		f.SetCellValue(sheetMatch, fmt.Sprintf("F%d", rowNum), r.MatchStatus)
		f.SetCellValue(sheetMatch, fmt.Sprintf("G%d", rowNum), r.MerchantPAN)
		f.SetCellValue(sheetMatch, fmt.Sprintf("H%d", rowNum), r.MerchantName)
		f.SetCellValue(sheetMatch, fmt.Sprintf("I%d", rowNum), r.MerchantCriteria)
		f.SetCellValue(sheetMatch, fmt.Sprintf("J%d", rowNum), r.InvoiceNumber)
		f.SetCellValue(sheetMatch, fmt.Sprintf("K%d", rowNum), r.CreatedDate)
		f.SetCellValue(sheetMatch, fmt.Sprintf("L%d", rowNum), r.CreatedTime)
		f.SetCellValue(sheetMatch, fmt.Sprintf("M%d", rowNum), r.ProcessingCode)
		f.SetCellValue(sheetMatch, fmt.Sprintf("N%d", rowNum), r.InterchangeFee)
		f.SetCellValue(sheetMatch, fmt.Sprintf("O%d", rowNum), r.ConvenienceFee)
	}

	// Hapus Sheet1 default
	f.DeleteSheet("Sheet1")

	// Save file
	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("gagal menyimpan file Excel: %w", err)
	}

	return nil
}
