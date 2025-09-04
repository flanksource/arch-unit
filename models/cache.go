package models

import (
	"time"
)

// FileMetadata represents metadata for cached files
type FileMetadata struct {
	ID              uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	FilePath        string    `json:"file_path" gorm:"column:file_path;uniqueIndex;not null"`
	FileHash        string    `json:"file_hash" gorm:"column:file_hash;not null;index"`
	FileSize        int64     `json:"file_size" gorm:"column:file_size"`
	LastModified    time.Time `json:"last_modified" gorm:"column:last_modified;not null;index"`
	LastAnalyzed    time.Time `json:"last_analyzed" gorm:"column:last_analyzed"`
	AnalysisVersion string    `json:"analysis_version" gorm:"column:analysis_version"`
}

// TableName specifies the table name for FileMetadata
func (FileMetadata) TableName() string {
	return "file_metadata"
}

// FileScan represents file scan records for violation cache
type FileScan struct {
	FilePath     string    `json:"file_path" gorm:"column:file_path;primaryKey"`
	LastScanTime int64     `json:"last_scan_time" gorm:"column:last_scan_time;not null"`
	FileModTime  int64     `json:"file_mod_time" gorm:"column:file_mod_time;not null"`
	FileHash     string    `json:"file_hash" gorm:"column:file_hash;not null"`
	Violations   []Violation `json:"violations,omitempty" gorm:"foreignKey:File;references:FilePath"`
}

// TableName specifies the table name for FileScan
func (FileScan) TableName() string {
	return "file_scans"
}