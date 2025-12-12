package service

import (
	"github.com/ciptami/switching-reconcile-web/internal/dto"
)

// ============================================================================
// FUNGSI PERBANDINGAN - COMPARISON FUNCTIONS
// ============================================================================

// CompareReconRRNs membandingkan RRN antara data CORE dan data Switching Reconciliation
// Menggunakan RRN sebagai key untuk matching
// Mengembalikan hasil perbandingan dengan status: MATCH, ONLY_IN_CORE, atau ONLY_IN_SWITCHING
func CompareReconRRNs(core []*dto.Data, switching map[string]dto.SwitchingReconciliationData) []dto.ReconciliationSwitchingResult {
	var results []dto.ReconciliationSwitchingResult
	coreMap := make(map[string]*dto.Data)
	
	// Buat hashmap dari data CORE dengan RRN sebagai key
	for _, data := range core {
		coreMap[data.RRN] = data
	}
	
	// Loop data switching dan bandingkan dengan CORE
	for rrn, switchData := range switching {
		if coreData, exists := coreMap[rrn]; exists {
			// MATCH - RRN ada di kedua sumber
			results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				Reff:             coreData.Reff,
				Status:           coreData.Status,
				MatchStatus:      "MATCH",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantName:     switchData.MerchantName,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessingCode:   switchData.ProcessingCode,
			})
			delete(coreMap, rrn)
		} else {
			// ONLY_IN_SWITCHING - RRN hanya ada di switching
			results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				MatchStatus:      "ONLY_IN_SWITCHING",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantName:     switchData.MerchantName,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessingCode:   switchData.ProcessingCode,
			})
		}
	}
	
	// Data yang tersisa di coreMap = ONLY_IN_CORE
	for rrn, coreData := range coreMap {
		results = append(results, dto.ReconciliationSwitchingResult{
			RRN:         rrn,
			Reff:        coreData.Reff,
			Status:      coreData.Status,
			MatchStatus: "ONLY_IN_CORE",
		})
	}
	
	return results
}

// SettlementComparisonResult holds comparison results with statistics
type SettlementComparisonResult struct {
	Results    []dto.SettlementSwitchingResult
	MatchCount int
}

// CompareSettlementRRNs membandingkan data settlement menggunakan composite key (RRN + Amount)
// Matching harus exact: RRN sama DAN Amount sama
// Return: ALL records (MATCH + ONLY_IN_CORE + ONLY_IN_SWITCHING) + MatchCount
func CompareSettlementRRNs(core []*dto.Data, switching map[string]dto.SwitchingSettlementData) ([]dto.SettlementSwitchingResult, int) {
	var results []dto.SettlementSwitchingResult
	coreMap := make(map[string]*dto.Data)
	matchCount := 0
	
	// Buat hashmap dari data CORE dengan composite key (RRN-Amount)
	// Format key sesuai Ciptami: "RRN-Amount" dengan 4 desimal
	for _, data := range core {
		key := data.Key() // Format: "RRN-Amount" (contoh: "000819948298-10000.0000")
		if coreMap[key] != nil {
			// Key duplikat di data CORE - skip
			continue
		}
		coreMap[key] = data
	}
	
	// Loop data switching dan bandingkan dengan CORE
	for key, switchData := range switching {
		if coreData, exists := coreMap[key]; exists {
			// MATCH - Include in results with MATCH status
			matchCount++
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              switchData.RRN,
				Amount:           switchData.Amount,
				Reff:             coreData.Reff,
				Status:           coreData.Status,
				MatchStatus:      "MATCH",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantName:     switchData.MerchantName,
				MerchantCriteria: switchData.MerchantCriteria,
				InvoiceNumber:    switchData.TraceNo,
				CreatedDate:      switchData.TanggalTrx,
				CreatedTime:      switchData.JamTrx,
				ProcessingCode:   switchData.TrxCode,
				InterchangeFee:   switchData.InterchangeFee,
				ConvenienceFee:   switchData.ConvenienceFee,
			})
			delete(coreMap, key)
		} else {
			// ONLY_IN_SWITCHING - Output record ini
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              switchData.RRN,
				Amount:           switchData.Amount,
				MatchStatus:      "ONLY_IN_SWITCHING",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantName:     switchData.MerchantName,
				MerchantCriteria: switchData.MerchantCriteria,
				InvoiceNumber:    switchData.TraceNo,
				CreatedDate:      switchData.TanggalTrx,
				CreatedTime:      switchData.JamTrx,
				ProcessingCode:   switchData.TrxCode,
				InterchangeFee:   switchData.InterchangeFee,
				ConvenienceFee:   switchData.ConvenienceFee,
			})
		}
	}
	
	// Data yang tersisa di coreMap = ONLY_IN_CORE - Output semua
	for _, coreData := range coreMap {
		results = append(results, dto.SettlementSwitchingResult{
			RRN:         coreData.RRN,
			Amount:      coreData.Amount,
			Reff:        coreData.Reff,
			Status:      coreData.Status,
			MatchStatus: "ONLY_IN_CORE",
		})
	}
	
	return results, matchCount
}
