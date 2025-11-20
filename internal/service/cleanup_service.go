package service

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CleanupService struct {
	resultsDir string
}

type FolderInfo struct {
	Name      string
	Path      string
	Date      time.Time
	DaysOld   int
	FileCount int
	SizeBytes int64
	SizeMB    float64
}

func NewCleanupService(resultsDir string) *CleanupService {
	return &CleanupService{
		resultsDir: resultsDir,
	}
}

// AutoCleanup - Hapus otomatis folder > retention days
func (s *CleanupService) AutoCleanup(retentionDays int) (int, error) {
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return 0, fmt.Errorf("gagal membaca direktori results: %w", err)
	}

	deletedCount := 0
	var totalSize int64

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "converted" {
			continue
		}

		folderDate, err := s.parseFolderDate(entry.Name())
		if err != nil {
			continue
		}

		if folderDate.Before(cutoffDate) {
			folderPath := filepath.Join(s.resultsDir, entry.Name())
			fileCount, sizeBytes := s.getFolderStats(folderPath)
			daysOld := int(time.Since(folderDate).Hours() / 24)

			// Hapus folder
			if err := os.RemoveAll(folderPath); err != nil {
				log.Printf("⚠️  Gagal menghapus %s: %v\n", entry.Name(), err)
				continue
			}

			deletedCount++
			totalSize += sizeBytes
			sizeMB := float64(sizeBytes) / (1024 * 1024)

			log.Printf("✅ Terhapus: %s (%d hari, %d files, %.2f MB)\n",
				entry.Name(), daysOld, fileCount, sizeMB)
		}
	}

	if deletedCount > 0 {
		totalSizeMB := float64(totalSize) / (1024 * 1024)
		log.Printf("🎉 Total: %d folder dihapus (%.2f MB dibebaskan)\n", deletedCount, totalSizeMB)
	}

	return deletedCount, nil
}

// GetOldFolders - Preview folder yang akan dihapus (untuk monitoring/logging)
func (s *CleanupService) GetOldFolders(retentionDays int) ([]FolderInfo, error) {
	var oldFolders []FolderInfo
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca direktori results: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "converted" {
			continue
		}

		folderDate, err := s.parseFolderDate(entry.Name())
		if err != nil {
			continue
		}

		if folderDate.Before(cutoffDate) {
			folderPath := filepath.Join(s.resultsDir, entry.Name())
			fileCount, sizeBytes := s.getFolderStats(folderPath)
			daysOld := int(time.Since(folderDate).Hours() / 24)

			oldFolders = append(oldFolders, FolderInfo{
				Name:      entry.Name(),
				Path:      folderPath,
				Date:      folderDate,
				DaysOld:   daysOld,
				FileCount: fileCount,
				SizeBytes: sizeBytes,
				SizeMB:    float64(sizeBytes) / (1024 * 1024),
			})
		}
	}

	return oldFolders, nil
}

// parseFolderDate - Parse tanggal dari nama folder (0001-18-11-2025)
func (s *CleanupService) parseFolderDate(folderName string) (time.Time, error) {
	parts := strings.Split(folderName, "-")
	if len(parts) != 4 {
		return time.Time{}, fmt.Errorf("format folder tidak valid")
	}

	// Format: 0001-DD-MM-YYYY -> YYYY-MM-DD
	dateStr := fmt.Sprintf("%s-%s-%s", parts[3], parts[2], parts[1])
	return time.Parse("2006-01-02", dateStr)
}

// getFolderStats - Hitung jumlah file dan total size dalam folder
func (s *CleanupService) getFolderStats(folderPath string) (fileCount int, totalSize int64) {
	filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})
	return
}
