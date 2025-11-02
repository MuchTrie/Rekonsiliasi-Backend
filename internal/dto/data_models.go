package dto

// Data represents core transaction data
type Data struct {
	Vendor       string
	RRN          string
	Reff         string
	ClientReff   string
	SupplierReff string
	Status       string
	CreatedDate  string
	CreatedTime  string
	PaidDate     string
	PaidTime     string
}

// SwitchingReconciliationData represents switching reconciliation data
type SwitchingReconciliationData struct {
	RRN            string
	MerchantPAN    string
	Criteria       string
	InvoiceNumber  string
	CreatedDate    string
	CreatedTime    string
	ProcessingCode string
}

// SwitchingSettlementData represents switching settlement data
type SwitchingSettlementData struct {
	RRN             string
	MerchantPAN     string
	MerchantCriteria string
	TraceNo         string
	TanggalTrx      string
	JamTrx          string
	TrxCode         string
	ConvenienceFee  string
	InterchangeFee  string
}

// ReconciliationCoreResult represents core result
type ReconciliationCoreResult struct {
	RRN              string `csv:"rrn"`
	Reff             string `csv:"reff"`
	Status           string `csv:"status"`
	MatchStatus      string `csv:"match_status"`
	InvoiceNumber    string `csv:"invoice_number"`
	MerchantCriteria string `csv:"merchant_criteria"`
	MerchantPAN      string `csv:"merchant_pan"`
	ProcessCode      string `csv:"process_code"`
	CreatedDate      string `csv:"created_date"`
	CreatedTime      string `csv:"created_time"`
}

// ReconciliationSwitchingResult represents reconciliation result
type ReconciliationSwitchingResult struct {
	RRN              string `csv:"rrn"`
	Reff             string `csv:"reff"`
	Status           string `csv:"status"`
	MatchStatus      string `csv:"match_status"`
	MerchantPAN      string `csv:"merchant_pan"`
	MerchantCriteria string `csv:"merchant_criteria"`
	InvoiceNumber    string `csv:"invoice_number"`
	CreatedDate      string `csv:"created_date"`
	CreatedTime      string `csv:"created_time"`
	ProcessingCode   string `csv:"processing_code"`
}

// SettlementSwitchingResult represents settlement result
type SettlementSwitchingResult struct {
	RRN              string `csv:"rrn"`
	Reff             string `csv:"reff"`
	Status           string `csv:"status"`
	MatchStatus      string `csv:"match_status"`
	MerchantPAN      string `csv:"merchant_pan"`
	MerchantCriteria string `csv:"merchant_criteria"`
	InvoiceNumber    string `csv:"invoice_number"`
	CreatedDate      string `csv:"created_date"`
	CreatedTime      string `csv:"created_time"`
	ProcessingCode   string `csv:"processing_code"`
	InterchangeFee   string `csv:"interchange_fee"`
	ConvenienceFee   string `csv:"convenience_fee"`
}
