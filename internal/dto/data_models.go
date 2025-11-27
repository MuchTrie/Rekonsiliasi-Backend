package dto

import "fmt"

// Data represents core transaction data
type Data struct {
	Vendor       string
	RRN          string
	Amount       float64
	Reff         string
	ClientReff   string
	SupplierReff string
	Status       string
	CreatedDate  string
	CreatedTime  string
	PaidDate     string
	PaidTime     string
}

// Key returns composite key for settlement comparison (RRN + Amount)
// Format: RRN-Amount with 4 decimal precision (sesuai format Ciptami)
func (d *Data) Key() string {
	return fmt.Sprintf("%s-%.4f", d.RRN, d.Amount)
}

// SwitchingReconciliationData represents switching reconciliation data
type SwitchingReconciliationData struct {
	RRN            string
	Amount         float64
	MerchantPAN    string
	Criteria       string
	InvoiceNumber  string
	CreatedDate    string
	CreatedTime    string
	ProcessingCode string
}

// Key returns composite key for settlement comparison (RRN + Amount)
// Format: RRN-Amount with 4 decimal precision (sesuai format Ciptami)
func (s SwitchingReconciliationData) Key() string {
	return fmt.Sprintf("%s-%.4f", s.RRN, s.Amount)
}

// SwitchingSettlementData represents switching settlement data
type SwitchingSettlementData struct {
	SwitchingReconciliationData // Embedded struct (inherit Amount and Key())
	TraceNo                     string
	TanggalTrx                  string
	JamTrx                      string
	TrxCode                     string
	MerchantCriteria            string
	ConvenienceFee              string
	InterchangeFee              string
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
	RRN              string  `csv:"rrn"`
	Amount           float64 `csv:"amount"`
	Reff             string  `csv:"reff"`
	Status           string  `csv:"status"`
	MatchStatus      string  `csv:"match_status"`
	MerchantPAN      string  `csv:"merchant_pan"`
	MerchantCriteria string  `csv:"merchant_criteria"`
	InvoiceNumber    string  `csv:"invoice_number"`
	CreatedDate      string  `csv:"created_date"`
	CreatedTime      string  `csv:"created_time"`
	ProcessingCode   string  `csv:"processing_code"`
	InterchangeFee   string  `csv:"interchange_fee"`
	ConvenienceFee   string  `csv:"convenience_fee"`
}
