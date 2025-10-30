package validator

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
)

// FileValidator melakukan validasi file upload
type FileValidator struct {
	AllowedExtensions map[string]bool
	MaxFileSizeMB     int64 // 0 = unlimited
}

// NewFileValidator membuat instance baru FileValidator
func NewFileValidator() *FileValidator {
	return &FileValidator{
		AllowedExtensions: map[string]bool{
			".csv": true,
			".txt": true,
			".bin": true,
		},
		MaxFileSizeMB: 0, // unlimited untuk sementara
	}
}

// ValidateFile melakukan validasi file yang diupload
func (v *FileValidator) ValidateFile(file *multipart.FileHeader) error {
	if file == nil {
		return fmt.Errorf("file tidak boleh kosong")
	}
	
	// Validasi ekstensi file
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !v.AllowedExtensions[ext] {
		return fmt.Errorf("ekstensi file %s tidak diizinkan. Hanya CSV, TXT, atau BIN", ext)
	}
	
	// Validasi ukuran file (jika ada limit)
	if v.MaxFileSizeMB > 0 {
		maxSize := v.MaxFileSizeMB * 1024 * 1024
		if file.Size > maxSize {
			return fmt.Errorf("ukuran file %d MB melebihi limit %d MB", 
				file.Size/(1024*1024), v.MaxFileSizeMB)
		}
	}
	
	return nil
}

// ValidateCSVFile melakukan validasi khusus untuk file CSV
func (v *FileValidator) ValidateCSVFile(file *multipart.FileHeader) error {
	if err := v.ValidateFile(file); err != nil {
		return err
	}
	
	// Buka file untuk validasi struktur
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("gagal membuka file: %w", err)
	}
	defer src.Close()
	
	// Cek apakah file CSV valid (minimal ada header)
	reader := csv.NewReader(src)
	reader.FieldsPerRecord = -1 // Allow variable fields
	
	_, err = reader.Read()
	if err != nil && err != io.EOF {
		return fmt.Errorf("file CSV tidak valid: %w", err)
	}
	
	return nil
}

// IsCSVFile mengecek apakah file adalah CSV
func (v *FileValidator) IsCSVFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".csv"
}

// IsBINFile mengecek apakah file adalah BIN/TXT (untuk settlement)
func (v *FileValidator) IsBINFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".bin" || ext == ".txt"
}
