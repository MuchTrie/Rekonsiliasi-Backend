package service

import (
	"bufio"
	"encoding/csv"
	"os"
	"strings"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// ============================================================================
// DATA EXTRACTOR - STRUCT & CONSTRUCTOR
// ============================================================================

// DataExtractor menangani ekstraksi data dari berbagai format file (CSV, TXT)
type DataExtractor struct {
	log *logrus.Logger
}

// NewDataExtractor membuat instance baru dari DataExtractor
func NewDataExtractor(log *logrus.Logger) *DataExtractor {
	return &DataExtractor{
		log: log,
	}
}

// ============================================================================
// FUNGSI EKSTRAKSI CORE DATA
// ============================================================================

// ExtractSingleCoreData mengekstrak data dari file CORE CSV
// Format CORE: No, Status, Settle, Created Date, ..., RRN (kolom 13), Supplier Name (kolom 14), Amount (kolom 15), ...
// Mengembalikan slice dari dto.Data dengan composite key (RRN + Amount)
func (de *DataExtractor) ExtractSingleCoreData(path string) ([]*dto.Data, error) {
	// Buka file CSV
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	// Setup CSV reader
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	
	// Baca semua records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	
	var result []*dto.Data
	
	// Loop records (skip header di index 0)
	for i, row := range records {
		if i == 0 {
			continue // Skip header row
		}
		
		// Validasi jumlah kolom (minimal 16 kolom)
		if len(row) < 16 {
			de.log.Warnf("Baris %d memiliki kolom tidak lengkap (%d kolom), dilewati", i, len(row))
			continue
		}
		
		// Extract RRN dari kolom 13 (index 13)
		rrn := strings.TrimSpace(row[13])
		if rrn == "" {
			continue // Skip jika RRN kosong
		}
		
		// PENTING: Skip record dengan status != "SUCCESS" (sesuai logic Ciptami)
		status := strings.TrimSpace(row[1])
		if strings.ToUpper(status) != "SUCCESS" {
			continue // Skip jika status bukan SUCCESS
		}
		
		// Parse Amount dari kolom 15 (index 15)
		amount := AmountConverter(row[15], de.log)
		
		// Buat struct Data dengan field yang diperlukan
		data := &dto.Data{
			RRN:          rrn,
			Amount:       amount,
			Reff:         strings.TrimSpace(row[10]),  // Reff di kolom 10
			ClientReff:   strings.TrimSpace(row[11]),  // Client Reff di kolom 11
			SupplierReff: strings.TrimSpace(row[12]),  // Supplier Reff di kolom 12
			Status:       strings.TrimSpace(row[1]),   // Status di kolom 1
			CreatedDate:  strings.TrimSpace(row[3]),   // Created Date di kolom 3
			CreatedTime:  strings.TrimSpace(row[4]),   // Created Time di kolom 4
			PaidDate:     strings.TrimSpace(row[5]),   // Paid Date di kolom 5
			PaidTime:     strings.TrimSpace(row[6]),   // Paid Time di kolom 6
		}
		
		// Extract Vendor Name (Supplier Name) jika ada
		if len(row) > 14 {
			data.Vendor = strings.TrimSpace(row[14]) // Supplier Name di kolom 14
		}
		
		result = append(result, data)
	}
	
	de.log.Infof("Berhasil ekstrak %d records dari CORE file", len(result))
	return result, nil
}

// ============================================================================
// FUNGSI EKSTRAKSI RECONCILIATION DATA
// ============================================================================

// ExtractReconciliationDataNew mengekstrak data rekonsiliasi dari file TXT yang sudah dikonversi ke CSV
// Format: Pipe-delimited TXT yang sudah dikonversi menjadi CSV dengan format tertentu
// Mengembalikan map dengan RRN sebagai key
func (de *DataExtractor) ExtractReconciliationDataNew(file *os.File) map[string]dto.SwitchingReconciliationData {
	// Setup CSV reader
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	
	// Baca semua records
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		de.log.Errorf("Gagal membaca file rekonsiliasi: %v", err)
		return make(map[string]dto.SwitchingReconciliationData)
	}
	
	result := make(map[string]dto.SwitchingReconciliationData)
	
	// Loop records dan extract data
	// Format actual file recon JALIN/ALTO (pipe-delimited):
	// DH|Terminal|Trace|MerchantPAN|Date|Time|ProcessCode|Amount|C/D|Fee|Category|Criteria|Acquirer|Issuer|SettlementID|ResponseCode|CustomerPAN|RefData|Stan|MTI
	// Index: 0   1        2     3           4    5    6           7      8   9   10       11       12       13     14           15           16         17      18   19
	// CATATAN: File hasil konversi tidak punya header row, langsung data
	// IMPORTANT: Trace Number (kolom 2) adalah RRN yang digunakan untuk matching dengan CORE!
	for i, row := range records {
		if len(row) < 17 {
			de.log.Warnf("Baris %d memiliki kolom tidak lengkap (%d kolom), dilewati", i+1, len(row))
			continue // Skip row dengan kolom tidak lengkap
		}
		
		// Extract Trace Number dari kolom 2 (digunakan sebagai RRN untuk matching dengan CORE)
		rrn := strings.TrimSpace(row[2])
		
		if rrn == "" {
			continue // Skip jika RRN kosong
		}
		
		// Check duplikasi
		if _, exists := result[rrn]; exists {
			de.log.Warnf("RRN duplikat ditemukan: %s", rrn)
			continue
		}
		
		// Extract dan convert Amount dari kolom 7
		// Format: 000002000000 (12 digit, 2 digit terakhir adalah desimal)
		// Contoh: 000002000000 = 20000.00
		amount := AmountConverter(row[7], de.log)
		
		// Buat struct SwitchingReconciliationData
		result[rrn] = dto.SwitchingReconciliationData{
			RRN:            rrn,
			Amount:         amount,
			MerchantPAN:    strings.TrimSpace(row[3]),
			Criteria:       strings.TrimSpace(row[11]),
			InvoiceNumber:  strings.TrimSpace(row[2]),  // Trace Number
			CreatedDate:    strings.TrimSpace(row[4]),
			CreatedTime:    strings.TrimSpace(row[5]),
			ProcessingCode: strings.TrimSpace(row[6]),
		}
	}
	
	de.log.Infof("Berhasil ekstrak %d records dari file rekonsiliasi", len(result))
	return result
}

// ============================================================================
// FUNGSI EKSTRAKSI SETTLEMENT DATA
// ============================================================================

// ExtractSettlementDataFromCSV mengekstrak data settlement dari file CSV (hasil konversi)
// Format CSV: No,Trx_Code,Tanggal_Trx,Jam_Trx,Ref_No,Trace_No,Terminal_ID,Merchant_PAN,...
// Menggunakan composite key (RRN + Amount) untuk matching
// Mengembalikan map dengan key = "RRN|Amount"
func (de *DataExtractor) ExtractSettlementDataFromCSV(file *os.File) map[string]dto.SwitchingSettlementData {
	// Setup CSV reader
	reader := csv.NewReader(file)
	reader.Comma = ','
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	
	// Baca semua records
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		de.log.Errorf("Gagal membaca file settlement CSV: %v", err)
		return make(map[string]dto.SwitchingSettlementData)
	}
	
	result := make(map[string]dto.SwitchingSettlementData)
	
	// Loop records (skip header di index 0)
	// Format: No,Trx_Code,Tanggal_Trx,Jam_Trx,Ref_No,Trace_No,Terminal_ID,Merchant_PAN,Acquirer,Issuer,Customer_PAN,Nominal,Merchant_Category,Merchant_Criteria,Response_Code,Merchant_Name_Location,Convenience_Fee,Interchange_Fee
	for i, row := range records {
		if i == 0 || len(row) < 18 {
			continue // Skip header atau row dengan kolom tidak lengkap
		}
		
		// Extract fields dari CSV
		rrn := strings.TrimSpace(row[4])  // Ref_No (column 4)
		if rrn == "" {
			continue
		}
		
		// Convert Amount dari string ke float64
		amount := AmountConverter(row[11], de.log) // Nominal (column 11)
		
		// Buat struct SettlementData
		settlementData := dto.SwitchingSettlementData{
			SwitchingReconciliationData: dto.SwitchingReconciliationData{
				RRN:            rrn,
				Amount:         amount,
				MerchantPAN:    strings.TrimSpace(row[7]),  // Merchant_PAN (column 7)
				Criteria:       strings.TrimSpace(row[13]), // Merchant_Criteria (column 13)
				InvoiceNumber:  strings.TrimSpace(row[5]),  // Trace_No (column 5)
				CreatedDate:    strings.TrimSpace(row[2]),  // Tanggal_Trx (column 2)
				CreatedTime:    strings.TrimSpace(row[3]),  // Jam_Trx (column 3)
				ProcessingCode: strings.TrimSpace(row[1]),  // Trx_Code (column 1)
			},
			TraceNo:          strings.TrimSpace(row[5]),  // Trace_No
			TanggalTrx:       strings.TrimSpace(row[2]),  // Tanggal_Trx
			JamTrx:           strings.TrimSpace(row[3]),  // Jam_Trx
			TrxCode:          strings.TrimSpace(row[1]),  // Trx_Code
			MerchantCriteria: strings.TrimSpace(row[13]), // Merchant_Criteria
			ConvenienceFee:   strings.TrimSpace(row[16]), // Convenience_Fee (column 16)
			InterchangeFee:   strings.TrimSpace(row[17]), // Interchange_Fee (column 17)
		}
		
		// Gunakan composite key (RRN + Amount) untuk matching yang akurat
		key := settlementData.Key() // Format: "RRN|Amount"
		if _, exists := result[key]; exists {
			de.log.Warnf("Key settlement duplikat ditemukan: %s", key)
			continue
		}
		
		result[key] = settlementData
	}
	
	de.log.Infof("Berhasil ekstrak %d records settlement dari CSV", len(result))
	return result
}

// ExtractSettlementData mengekstrak data settlement dari file TXT (backward compatibility)
// Format Settlement: 1 transaksi = 1 baris panjang (500+ karakter) dengan semua field
// Menggunakan composite key (RRN + Amount) untuk matching
// Mengembalikan map dengan key = "RRN|Amount"
func (de *DataExtractor) ExtractSettlementData(file *os.File) map[string]dto.SwitchingSettlementData {
	// Setup scanner dengan buffer besar untuk baris panjang
	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)
	
	result := make(map[string]dto.SwitchingSettlementData)
	
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
		
		// Skip baris kosong dan header/footer
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
		if len(line) > 50 && line[0] >= '0' && line[0] <= '9' {
			// Gunakan parser single-line untuk extract semua field
			parsed := ParseSettlementDataLineSingleLine(line)
			if parsed == nil {
				continue
			}
			
			// Extract RRN dan Amount
			rrn := parsed["Ref_No"]
			if rrn == "" {
				continue
			}
			
			// Convert Amount dari string ke float64
			amount := AmountConverter(parsed["Nominal"], de.log)
			
			// Buat struct SettlementData
			settlementData := dto.SwitchingSettlementData{
				SwitchingReconciliationData: dto.SwitchingReconciliationData{
					RRN:            rrn,
					Amount:         amount,
					MerchantPAN:    parsed["Merchant_PAN"],
					Criteria:       parsed["Merchant_Criteria"],
					InvoiceNumber:  parsed["Trace_No"],
					CreatedDate:    parsed["Tanggal_Trx"],
					CreatedTime:    parsed["Jam_Trx"],
					ProcessingCode: parsed["Trx_Code"],
				},
				TraceNo:          parsed["Trace_No"],
				TanggalTrx:       parsed["Tanggal_Trx"],
				JamTrx:           parsed["Jam_Trx"],
				TrxCode:          parsed["Trx_Code"],
				MerchantCriteria: parsed["Merchant_Criteria"],
				ConvenienceFee:   parsed["Convenience_Fee"],
				InterchangeFee:   parsed["Interchange_Fee"],
			}
			
			// Gunakan composite key (RRN + Amount) untuk matching yang akurat
			key := settlementData.Key() // Format: "RRN|Amount" (contoh: "1iefp2w46282|20000.00")
			if _, exists := result[key]; exists {
				de.log.Warnf("Key settlement duplikat ditemukan: %s", key)
				continue
			}
			
			result[key] = settlementData
		}
	}
	
	de.log.Infof("Berhasil ekstrak %d records settlement", len(result))
	return result
}
