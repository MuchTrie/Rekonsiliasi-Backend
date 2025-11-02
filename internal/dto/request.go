package dto

import (
	"mime/multipart"
	"strings"
)

// ReconciliationRequest adalah request untuk proses rekonsiliasi
type ReconciliationRequest struct {
	// Multiple CORE files - sistem akan auto-detect vendor dari nama file
	CoreFiles []*multipart.FileHeader `form:"core_files" binding:"required"`
	
	// Recon files - multiple files per vendor (array)
	AltoReconFiles  []*multipart.FileHeader `form:"alto_recon_files"`
	JalinReconFiles []*multipart.FileHeader `form:"jalin_recon_files"`
	AJReconFiles    []*multipart.FileHeader `form:"aj_recon_files"`
	RintiReconFiles []*multipart.FileHeader `form:"rinti_recon_files"`
	
	// Settlement files - multiple files per vendor (array)
	AltoSettlementFiles  []*multipart.FileHeader `form:"alto_settlement_files"`
	JalinSettlementFiles []*multipart.FileHeader `form:"jalin_settlement_files"`
	AJSettlementFiles    []*multipart.FileHeader `form:"aj_settlement_files"`
	RintiSettlementFiles []*multipart.FileHeader `form:"rinti_settlement_files"`
}

// VendorFiles menyimpan file-file untuk satu vendor
type VendorFiles struct {
	Vendor          string
	CoreFile        *multipart.FileHeader   // CORE file untuk vendor ini
	ReconFiles      []*multipart.FileHeader // Multiple recon files
	SettlementFiles []*multipart.FileHeader // Multiple settlement files
}

// GetVendorFilesMap mengelompokkan files berdasarkan vendor yang terdeteksi dari CORE files
func (r *ReconciliationRequest) GetVendorFilesMap() map[string]*VendorFiles {
	vendorMap := make(map[string]*VendorFiles)
	
	// Detect vendor from each CORE file and group
	for _, coreFile := range r.CoreFiles {
		vendor := detectVendorFromFilename(coreFile.Filename)
		if vendor == "" {
			continue // Skip jika tidak bisa detect vendor
		}
		
		if _, exists := vendorMap[vendor]; !exists {
			vendorMap[vendor] = &VendorFiles{
				Vendor:          vendor,
				ReconFiles:      []*multipart.FileHeader{},
				SettlementFiles: []*multipart.FileHeader{},
			}
		}
		
		vendorMap[vendor].CoreFile = coreFile
	}
	
	// Assign recon files
	if len(r.AltoReconFiles) > 0 {
		if vendorMap["alto"] == nil {
			vendorMap["alto"] = &VendorFiles{Vendor: "alto", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["alto"].ReconFiles = r.AltoReconFiles
	}
	
	if len(r.JalinReconFiles) > 0 {
		if vendorMap["jalin"] == nil {
			vendorMap["jalin"] = &VendorFiles{Vendor: "jalin", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["jalin"].ReconFiles = r.JalinReconFiles
	}
	
	if len(r.AJReconFiles) > 0 {
		if vendorMap["aj"] == nil {
			vendorMap["aj"] = &VendorFiles{Vendor: "aj", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["aj"].ReconFiles = r.AJReconFiles
	}
	
	if len(r.RintiReconFiles) > 0 {
		if vendorMap["rinti"] == nil {
			vendorMap["rinti"] = &VendorFiles{Vendor: "rinti", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["rinti"].ReconFiles = r.RintiReconFiles
	}
	
	// Assign settlement files
	if len(r.AltoSettlementFiles) > 0 {
		if vendorMap["alto"] == nil {
			vendorMap["alto"] = &VendorFiles{Vendor: "alto", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["alto"].SettlementFiles = r.AltoSettlementFiles
	}
	
	if len(r.JalinSettlementFiles) > 0 {
		if vendorMap["jalin"] == nil {
			vendorMap["jalin"] = &VendorFiles{Vendor: "jalin", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["jalin"].SettlementFiles = r.JalinSettlementFiles
	}
	
	if len(r.AJSettlementFiles) > 0 {
		if vendorMap["aj"] == nil {
			vendorMap["aj"] = &VendorFiles{Vendor: "aj", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["aj"].SettlementFiles = r.AJSettlementFiles
	}
	
	if len(r.RintiSettlementFiles) > 0 {
		if vendorMap["rinti"] == nil {
			vendorMap["rinti"] = &VendorFiles{Vendor: "rinti", ReconFiles: []*multipart.FileHeader{}, SettlementFiles: []*multipart.FileHeader{}}
		}
		vendorMap["rinti"].SettlementFiles = r.RintiSettlementFiles
	}
	
	return vendorMap
}

// detectVendorFromFilename detects vendor name from filename
// Looks for keywords: ALTO, JALIN, AJ, RINTI (case-insensitive)
func detectVendorFromFilename(filename string) string {
	lower := strings.ToLower(filename)
	
	if strings.Contains(lower, "alto") {
		return "alto"
	}
	if strings.Contains(lower, "jalin") {
		return "jalin"
	}
	if strings.Contains(lower, "aj") && !strings.Contains(lower, "jalin") {
		return "aj"
	}
	if strings.Contains(lower, "rinti") {
		return "rinti"
	}
	
	return "" // Unknown vendor
}
