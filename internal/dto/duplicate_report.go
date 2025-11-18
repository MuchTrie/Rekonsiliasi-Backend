package dto

// DuplicateRecord represents a single duplicate transaction record
type DuplicateRecord struct {
	RRN         string  `json:"rrn"`
	Amount      float64 `json:"amount"`
	LineNumber  int     `json:"line_number"`
	Source      string  `json:"source"`       // "CORE", "SWITCHING_RECON", "SWITCHING_SETTLEMENT"
	FileName    string  `json:"file_name"`
	Vendor      string  `json:"vendor"`       // "ALTO", "JALIN", "AJ", "RINTI"
	CreatedDate string  `json:"created_date"`
	CreatedTime string  `json:"created_time"`
}

// DuplicateGroup represents a group of duplicate records with the same RRN
type DuplicateGroup struct {
	RRN             string            `json:"rrn"`
	OccurrenceCount int               `json:"occurrence_count"` // Jumlah kemunculan
	Records         []DuplicateRecord `json:"records"`          // List semua record dengan RRN sama
	TotalAmount     float64           `json:"total_amount"`     // Total amount dari semua duplicate
}

// DuplicateReport represents the complete duplicate detection report
type DuplicateReport struct {
	JobID             string           `json:"job_id"`
	TotalDuplicates   int              `json:"total_duplicates"`    // Total jumlah RRN yang duplicate
	TotalRecords      int              `json:"total_records"`       // Total semua duplicate records
	CoreDuplicates    []DuplicateGroup `json:"core_duplicates"`     // Duplicate di CORE files
	ReconDuplicates   []DuplicateGroup `json:"recon_duplicates"`    // Duplicate di Reconciliation files
	SettleDuplicates  []DuplicateGroup `json:"settle_duplicates"`   // Duplicate di Settlement files
	GeneratedAt       string           `json:"generated_at"`
}

// DuplicateStats provides summary statistics
type DuplicateStats struct {
	Source          string `json:"source"`           // "CORE", "RECON", "SETTLEMENT"
	Vendor          string `json:"vendor"`
	TotalDuplicates int    `json:"total_duplicates"` // Jumlah RRN unique yang duplicate
	TotalRecords    int    `json:"total_records"`    // Total baris duplicate
}
