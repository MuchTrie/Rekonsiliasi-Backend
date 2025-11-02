package service

import (
	"bufio"
	"encoding/csv"
	"os"
	"strings"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// DataExtractor handles data extraction from various file formats
type DataExtractor struct {
	log *logrus.Logger
}

// NewDataExtractor creates a new data extractor
func NewDataExtractor(log *logrus.Logger) *DataExtractor {
	return &DataExtractor{
		log: log,
	}
}

// ExtractSingleCoreData mengekstrak data dari satu CORE file dengan format baru
func (de *DataExtractor) ExtractSingleCoreData(path string) ([]*dto.Data, error) {
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
			de.log.Warnf("Row %d has insufficient columns (%d), skipping", i, len(row))
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

// ExtractReconciliationDataNew mengekstrak data rekonsiliasi dari format baru (pipe-delimited TXT converted to CSV)
func (de *DataExtractor) ExtractReconciliationDataNew(file *os.File) map[string]dto.SwitchingReconciliationData {
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1
	
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		de.log.Errorf("Failed to read reconciliation file: %v", err)
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
			de.log.Warnf("Duplicate RRN found: %s", rrn)
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
	
	de.log.Infof("Loaded %d records from new recon file", len(result))
	return result
}

// ExtractSettlementData mengekstrak data settlement dari file
func (de *DataExtractor) ExtractSettlementData(file *os.File) map[string]dto.SwitchingSettlementData {
	scanner := bufio.NewScanner(file)
	result := make(map[string]dto.SwitchingSettlementData)
	
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
		
		// Skip empty lines and headers
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
		parsed := ParseSettlementDataLine(line)
		if parsed == nil {
			continue
		}
		
		rrn := parsed["Ref_No"]
		if rrn == "" {
			continue
		}
		
		result[rrn] = dto.SwitchingSettlementData{
			RRN:             rrn,
			MerchantPAN:     parsed["Merchant_PAN"],
			MerchantCriteria: parsed["Merchant_Criteria"],
			TraceNo:         parsed["Trace_No"],
			TanggalTrx:      parsed["Tanggal_Trx"],
			JamTrx:          parsed["Jam_Trx"],
			TrxCode:         parsed["Trx_Code"],
			ConvenienceFee:  parsed["Convenience_Fee"],
			InterchangeFee:  parsed["Interchange_Fee"],
		}
	}
	
	return result
}
