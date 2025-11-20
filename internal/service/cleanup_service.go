package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CleanupService handles cleanup of old reconciliation results
type CleanupService struct {
	resultsDir string
	log        *logrus.Logger
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(resultsDir string, log *logrus.Logger) *CleanupService {
	return &CleanupService{
		resultsDir: resultsDir,
		log:        log,
	}
}

// OldFolder represents a folder that is older than retention period
type OldFolder struct {
	Name     string
	Path     string
	Age      int // days
	FileCount int
}

// CheckOldFolders checks for folders older than the specified days
func (s *CleanupService) CheckOldFolders(retentionDays int) ([]OldFolder, error) {
	var oldFolders []OldFolder

	// Read all folders in results directory
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca direktori results: %v", err)
	}

	now := time.Now()

	for _, entry := range entries {
		// Skip if not a directory
		if !entry.IsDir() {
			continue
		}

		folderName := entry.Name()

		// Skip special folders
		if folderName == "converted" || folderName == "temp" {
			continue
		}

		// Parse date from folder name (format: 0001-18-11-2025)
		folderDate, err := s.parseFolderDate(folderName)
		if err != nil {
			s.log.Warnf("Tidak dapat parse tanggal dari folder %s: %v", folderName, err)
			continue
		}

		// Calculate age in days
		age := int(now.Sub(folderDate).Hours() / 24)

		// Check if older than retention period
		if age > retentionDays {
			folderPath := filepath.Join(s.resultsDir, folderName)
			
			// Count files in folder
			fileCount, err := s.countFiles(folderPath)
			if err != nil {
				s.log.Warnf("Tidak dapat hitung file di folder %s: %v", folderName, err)
				fileCount = 0
			}

			oldFolders = append(oldFolders, OldFolder{
				Name:      folderName,
				Path:      folderPath,
				Age:       age,
				FileCount: fileCount,
			})
		}
	}

	return oldFolders, nil
}

// DeleteFolders deletes the specified folders
func (s *CleanupService) DeleteFolders(folders []OldFolder) error {
	var totalSize int64
	deletedCount := 0

	for _, folder := range folders {
		// Calculate folder size before deletion
		size, err := s.getFolderSize(folder.Path)
		if err != nil {
			s.log.Warnf("Tidak dapat hitung ukuran folder %s: %v", folder.Name, err)
		}
		totalSize += size

		// Delete the folder
		err = os.RemoveAll(folder.Path)
		if err != nil {
			s.log.Errorf("❌ Gagal menghapus folder %s: %v", folder.Name, err)
			return fmt.Errorf("gagal menghapus folder %s: %v", folder.Name, err)
		}

		s.log.Infof("✅ Menghapus folder %s...", folder.Name)
		deletedCount++
	}

	// Convert size to MB
	sizeMB := float64(totalSize) / (1024 * 1024)
	s.log.Infof("🎉 Berhasil menghapus %d folder (mengosongkan %.2f MB)", deletedCount, sizeMB)

	return nil
}

// parseFolderDate parses date from folder name (format: 0001-18-11-2025)
func (s *CleanupService) parseFolderDate(folderName string) (time.Time, error) {
	parts := strings.Split(folderName, "-")
	if len(parts) != 4 {
		return time.Time{}, fmt.Errorf("format nama folder tidak valid: %s", folderName)
	}

	// Parts: [0001, 18, 11, 2025]
	day := parts[1]
	month := parts[2]
	year := parts[3]

	// Parse to time
	dateStr := fmt.Sprintf("%s-%s-%s", year, month, day)
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("gagal parse tanggal %s: %v", dateStr, err)
	}

	return date, nil
}

// countFiles counts the number of files in a directory
func (s *CleanupService) countFiles(dirPath string) (int, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}

	return count, nil
}

// getFolderSize calculates the total size of a folder
func (s *CleanupService) getFolderSize(dirPath string) (int64, error) {
	var size int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}
