package service

import (
	"github.com/ciptami/switching-reconcile-web/internal/dto"
)

// CompareReconRRNs membandingkan RRN antara core dan switching
func CompareReconRRNs(core []*dto.Data, switching map[string]dto.SwitchingReconciliationData) []dto.ReconciliationSwitchingResult {
	var results []dto.ReconciliationSwitchingResult
	coreMap := make(map[string]*dto.Data)
	
	for _, data := range core {
		coreMap[data.RRN] = data
	}
	
	// RRN exists in both
	for rrn, switchData := range switching {
		if coreData, exists := coreMap[rrn]; exists {
			results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				Reff:             coreData.Reff,
				Status:           coreData.Status,
				MatchStatus:      "MATCH",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessingCode:   switchData.ProcessingCode,
			})
			delete(coreMap, rrn)
		} else {
			results = append(results, dto.ReconciliationSwitchingResult{
				RRN:              rrn,
				MatchStatus:      "ONLY_IN_SWITCHING",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantCriteria: switchData.Criteria,
				InvoiceNumber:    switchData.InvoiceNumber,
				CreatedDate:      switchData.CreatedDate,
				CreatedTime:      switchData.CreatedTime,
				ProcessingCode:   switchData.ProcessingCode,
			})
		}
	}
	
	// RRN only in core
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

// CompareSettlementRRNs membandingkan settlement menggunakan composite key (RRN + Amount)
func CompareSettlementRRNs(core []*dto.Data, switching map[string]dto.SwitchingSettlementData) []dto.SettlementSwitchingResult {
	var results []dto.SettlementSwitchingResult
	coreKeys := make(map[string]bool)
	
	// Build map of core keys
	for _, data := range core {
		key := data.Key()
		if coreKeys[key] {
			// Duplicate key in core data - log but continue
			continue
		}
		coreKeys[key] = true
	}
	
	// Check switching data against core
	for key, switchData := range switching {
		if coreKeys[key] {
			// MATCH - RRN + Amount exists in both
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              switchData.RRN,
				Amount:           switchData.Amount,
				MatchStatus:      "MATCH",
				MerchantPAN:      switchData.MerchantPAN,
				MerchantCriteria: switchData.MerchantCriteria,
				InvoiceNumber:    switchData.TraceNo,
				CreatedDate:      switchData.TanggalTrx,
				CreatedTime:      switchData.JamTrx,
				ProcessingCode:   switchData.TrxCode,
				InterchangeFee:   switchData.InterchangeFee,
				ConvenienceFee:   switchData.ConvenienceFee,
			})
			delete(coreKeys, key)
		} else {
			// ONLY_IN_SWITCHING
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              switchData.RRN,
				Amount:           switchData.Amount,
				MatchStatus:      "ONLY_IN_SWITCHING",
				MerchantPAN:      switchData.MerchantPAN,
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
	
	// Remaining core keys are ONLY_IN_CORE
	for _, coreData := range core {
		key := coreData.Key()
		if coreKeys[key] {
			results = append(results, dto.SettlementSwitchingResult{
				RRN:         coreData.RRN,
				Amount:      coreData.Amount,
				Reff:        coreData.Reff,
				Status:      coreData.Status,
				MatchStatus: "ONLY_IN_CORE",
			})
		}
	}
	
	return results
}
