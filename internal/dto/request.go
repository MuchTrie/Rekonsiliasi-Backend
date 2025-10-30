package dto

import "mime/multipart"

// ReconciliationRequest adalah request untuk proses rekonsiliasi
type ReconciliationRequest struct {
	CoreFile *multipart.FileHeader `form:"core_file" binding:"required"`
	
	// Recon files - optional, tergantung vendor mana yang diupload
	AltoReconFile  *multipart.FileHeader `form:"alto_recon_file"`
	JalinReconFile *multipart.FileHeader `form:"jalin_recon_file"`
	AJReconFile    *multipart.FileHeader `form:"aj_recon_file"`
	RintiReconFile *multipart.FileHeader `form:"rinti_recon_file"`
	
	// Settlement files - optional
	AltoSettlementFile  *multipart.FileHeader `form:"alto_settlement_file"`
	JalinSettlementFile *multipart.FileHeader `form:"jalin_settlement_file"`
	AJSettlementFile    *multipart.FileHeader `form:"aj_settlement_file"`
	RintiSettlementFile *multipart.FileHeader `form:"rinti_settlement_file"`
}

// VendorFiles menyimpan file-file untuk satu vendor
type VendorFiles struct {
	Vendor         string
	ReconFile      *multipart.FileHeader
	SettlementFile *multipart.FileHeader
}

// GetVendorFiles mengekstrak vendor files dari request
func (r *ReconciliationRequest) GetVendorFiles() []VendorFiles {
	var vendors []VendorFiles
	
	if r.AltoReconFile != nil || r.AltoSettlementFile != nil {
		vendors = append(vendors, VendorFiles{
			Vendor:         "alto",
			ReconFile:      r.AltoReconFile,
			SettlementFile: r.AltoSettlementFile,
		})
	}
	
	if r.JalinReconFile != nil || r.JalinSettlementFile != nil {
		vendors = append(vendors, VendorFiles{
			Vendor:         "jalin",
			ReconFile:      r.JalinReconFile,
			SettlementFile: r.JalinSettlementFile,
		})
	}
	
	if r.AJReconFile != nil || r.AJSettlementFile != nil {
		vendors = append(vendors, VendorFiles{
			Vendor:         "aj",
			ReconFile:      r.AJReconFile,
			SettlementFile: r.AJSettlementFile,
		})
	}
	
	if r.RintiReconFile != nil || r.RintiSettlementFile != nil {
		vendors = append(vendors, VendorFiles{
			Vendor:         "rinti",
			ReconFile:      r.RintiReconFile,
			SettlementFile: r.RintiSettlementFile,
		})
	}
	
	return vendors
}
