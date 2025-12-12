package dto

import "time"

// APIResponse adalah format standar response API
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ReconciliationResult adalah hasil proses rekonsiliasi
type ReconciliationResult struct {
	JobID           string                  `json:"job_id"`
	Status          string                  `json:"status"` // processing, completed, failed
	Message         string                  `json:"message"`
	ProcessedAt     time.Time               `json:"processed_at"`
	TotalRecords    int                     `json:"total_records"`
	Vendors         []VendorResult          `json:"vendors"`
	DownloadURLs    map[string]string       `json:"download_urls,omitempty"`
	DuplicateReport *DuplicateReport        `json:"duplicate_report,omitempty"`
}

// VendorResult adalah hasil rekonsiliasi per vendor
type VendorResult struct {
	Vendor               string                `json:"vendor"`
	ReconResults         []ReconciliationData  `json:"recon_results,omitempty"`
	SettlementResults    []SettlementData      `json:"settlement_results,omitempty"`
	ReconMatchCount      int                   `json:"recon_match_count"`
	ReconMismatchCount   int                   `json:"recon_mismatch_count"`
	SettlementMatchCount int                   `json:"settlement_match_count"`
	SettlementMismatchCount int                `json:"settlement_mismatch_count"`
}

// ReconciliationData adalah data hasil rekonsiliasi (untuk ditampilkan di web)
type ReconciliationData struct {
	RRN              string `json:"rrn"`
	Reff             string `json:"reff"`
	Status           string `json:"status"`
	MerchantPAN      string `json:"merchant_pan"`
	MerchantName     string `json:"merchant_name"`
	MerchantCriteria string `json:"merchant_criteria"`
	InvoiceNumber    string `json:"invoice_number"`
	CreatedDate      string `json:"created_date"`
	CreatedTime      string `json:"created_time"`
	ProcessCode      string `json:"process_code"`
	MatchStatus      string `json:"match_status"` // MATCH, ONLY_IN_CORE, ONLY_IN_SWITCHING
	Source           string `json:"source"`       // CORE, SWITCHING, BOTH
}

// SettlementData adalah data hasil settlement
type SettlementData struct {
	RRN              string `json:"rrn"`
	Reff             string `json:"reff"`
	Status           string `json:"status"`
	MerchantPAN      string `json:"merchant_pan"`
	MerchantName     string `json:"merchant_name"`
	SettlementAmount string `json:"settlement_amount"`
	InterchangeFee   string `json:"interchange_fee"`
	ConvenienceFee   string `json:"convenience_fee"`
	MatchStatus      string `json:"match_status"`
	Source           string `json:"source"`
}

// FileInfo adalah informasi file yang diupload
type FileInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
}

// UploadResponse adalah response setelah upload file
type UploadResponse struct {
	JobID     string              `json:"job_id"`
	CoreFile  FileInfo            `json:"core_file"`
	Vendors   map[string]FileInfo `json:"vendors"`
	Message   string              `json:"message"`
}

// SettlementConversionResult adalah hasil konversi settlement file
type SettlementConversionResult struct {
	Filename       string                   `json:"filename"`
	TotalRecords   int                      `json:"total_records"`
	PreviewRecords []map[string]interface{} `json:"preview_records"`
	DownloadURL    string                   `json:"download_url"`
}

