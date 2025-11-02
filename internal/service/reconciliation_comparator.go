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

// CompareSettlementRRNs membandingkan settlement RRNs
func CompareSettlementRRNs(core []*dto.Data, switching map[string]dto.SwitchingSettlementData) []dto.SettlementSwitchingResult {
	var results []dto.SettlementSwitchingResult
	coreMap := make(map[string]*dto.Data)
	
	for _, data := range core {
		coreMap[data.RRN] = data
	}
	
	for rrn, switchData := range switching {
		if coreData, exists := coreMap[rrn]; exists {
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              rrn,
				Reff:             coreData.Reff,
				Status:           coreData.Status,
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
			delete(coreMap, rrn)
		} else {
			results = append(results, dto.SettlementSwitchingResult{
				RRN:              rrn,
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
	
	for rrn, coreData := range coreMap {
		results = append(results, dto.SettlementSwitchingResult{
			RRN:         rrn,
			Reff:        coreData.Reff,
			Status:      coreData.Status,
			MatchStatus: "ONLY_IN_CORE",
		})
	}
	
	return results
}
