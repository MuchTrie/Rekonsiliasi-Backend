package service

import (
	"encoding/csv"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ciptami/switching-reconcile-web/internal/dto"
	"github.com/sirupsen/logrus"
)

// SettlementConverter handles settlement file conversion
type SettlementConverter struct {
	log           *logrus.Logger
	uploadDir     string
	resultsDir    string
	fileConverter *FileConverter
}

// NewSettlementConverter creates a new settlement converter
func NewSettlementConverter(log *logrus.Logger, uploadDir, resultsDir string) *SettlementConverter {
	return &SettlementConverter{
		log:           log,
		uploadDir:     uploadDir,
		resultsDir:    resultsDir,
		fileConverter: NewFileConverter(log),
	}
}

// ConvertSettlementFile converts settlement TXT file to CSV and returns preview
func (sc *SettlementConverter) ConvertSettlementFile(file *multipart.FileHeader) (*dto.SettlementConversionResult, error) {
	// Create converted directory for settlement conversion results
	convertedDir := filepath.Join(sc.resultsDir, "converted")
	if err := os.MkdirAll(convertedDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create converted directory: %w", err)
	}
	
	// Create temp directory for temporary upload
	tempDir := filepath.Join(sc.uploadDir, "temp")
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	// Generate unique filename for converted CSV
	timestamp := time.Now().Format("20060102_150405")
	baseFilename := strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	csvFilename := fmt.Sprintf("%s_converted_%s.csv", baseFilename, timestamp)
	csvPath := filepath.Join(convertedDir, csvFilename)
	
	// Save uploaded TXT file temporarily
	txtPath := filepath.Join(tempDir, file.Filename)
	if err := saveUploadedFile(file, txtPath); err != nil {
		return nil, fmt.Errorf("failed to save uploaded file: %w", err)
	}
	
	// Convert TXT to CSV
	if err := sc.fileConverter.ConvertSettlementTxtToCsv(txtPath, csvPath); err != nil {
		os.Remove(txtPath) // Clean up
		return nil, fmt.Errorf("failed to convert settlement file: %w", err)
	}
	
	// Clean up temp TXT file
	os.Remove(txtPath)
	
	// Read CSV to get total records and preview
	csvFile, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted CSV: %w", err)
	}
	defer csvFile.Close()
	
	reader := csv.NewReader(csvFile)
	
	// Read header
	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}
	
	// Read all records for count
	allRecords, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV records: %w", err)
	}
	
	totalRecords := len(allRecords)
	
	// Build preview (limit to 100 records to prevent browser overflow)
	previewLimit := 100
	if totalRecords < previewLimit {
		previewLimit = totalRecords
	}
	
	previewRecords := make([]map[string]interface{}, 0, previewLimit)
	for i := 0; i < previewLimit; i++ {
		record := make(map[string]interface{})
		for j, header := range headers {
			if j < len(allRecords[i]) {
				record[header] = allRecords[i][j]
			}
		}
		previewRecords = append(previewRecords, record)
	}
	
	// Build download URL
	downloadURL := fmt.Sprintf("/api/download/converted/%s", csvFilename)
	
	return &dto.SettlementConversionResult{
		Filename:       csvFilename,
		TotalRecords:   totalRecords,
		PreviewRecords: previewRecords,
		DownloadURL:    downloadURL,
	}, nil
}

// GetConvertedFiles returns list of previously converted settlement files
func (sc *SettlementConverter) GetConvertedFiles() ([]map[string]interface{}, error) {
	convertedDir := filepath.Join(sc.resultsDir, "converted")
	
	// Create directory if not exists
	if err := os.MkdirAll(convertedDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create converted directory: %w", err)
	}
	
	// Read directory contents
	entries, err := os.ReadDir(convertedDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted directory: %w", err)
	}
	
	files := make([]map[string]interface{}, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}
		
		info, err := entry.Info()
		if err != nil {
			sc.log.Warnf("Failed to get file info for %s: %v", entry.Name(), err)
			continue
		}
		
		files = append(files, map[string]interface{}{
			"filename":     entry.Name(),
			"size":         info.Size(),
			"modified_at":  info.ModTime(),
			"download_url": fmt.Sprintf("/api/download/converted/%s", entry.Name()),
		})
	}
	
	// Sort by modified time (newest first)
	sort.Slice(files, func(i, j int) bool {
		timeI := files[i]["modified_at"].(time.Time)
		timeJ := files[j]["modified_at"].(time.Time)
		return timeI.After(timeJ)
	})
	
	return files, nil
}
