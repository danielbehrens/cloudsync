package sync

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Storage defines the interface for cloud storage operations
type Storage interface {
	Upload(ctx context.Context, localPath, objectName string) error
	Download(ctx context.Context, objectName, localPath string) error
	Stat(ctx context.Context, objectName string) (*SyncFileInfo, error)
	List(ctx context.Context) ([]*SyncFileInfo, error)
	EnsureBucket(ctx context.Context) error
}

// SyncFileInfo represents file metadata
type SyncFileInfo struct {
	Name    string
	ModTime time.Time
	Size    int64
}

// Syncer handles bidirectional file synchronization
type Syncer struct {
	storage       Storage
	watchPath     string
	backupDir     string
	processName   string
	timeTolerance time.Duration
}

// NewSyncer creates a new Syncer instance
func NewSyncer(storage Storage, watchPath, backupDir, processName string, timeTolerance time.Duration) *Syncer {
	return &Syncer{
		storage:       storage,
		watchPath:     watchPath,
		backupDir:     backupDir,
		processName:   processName,
		timeTolerance: timeTolerance,
	}
}

// IsProcessRunning checks if the specified process is currently running
func (s *Syncer) IsProcessRunning() bool {
	if s.processName == "" {
		return false
	}

	processes, err := process.Processes()
	if err != nil {
		log.Printf("Error listing processes: %v", err)
		return false
	}

	processName := strings.ToLower(s.processName)

	for _, p := range processes {
		exeName, err := p.Name()
		if err == nil && strings.Contains(strings.ToLower(exeName), processName) {
			return true
		}
	}
	return false
}

// InitialSync performs initial bidirectional synchronization
func (s *Syncer) InitialSync(ctx context.Context) error {
	log.Println("Starting initial sync...")

	// Ensure bucket exists
	if err := s.storage.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("failed to ensure bucket: %w", err)
	}

	// Upload newer local files
	if err := s.uploadLocalFiles(ctx); err != nil {
		return fmt.Errorf("failed to upload local files: %w", err)
	}

	// Download newer cloud files
	if err := s.downloadCloudFiles(ctx); err != nil {
		return fmt.Errorf("failed to download cloud files: %w", err)
	}

	log.Println("Initial sync complete")
	return nil
}

// SyncFile synchronizes a single file with the cloud
func (s *Syncer) SyncFile(ctx context.Context, filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}

	objectName := filepath.Base(filePath)

	// Check if file exists in cloud
	cloudInfo, err := s.storage.Stat(ctx, objectName)
	if err != nil {
		// File doesn't exist in cloud, upload it
		log.Printf("File %s not found in cloud, uploading...", objectName)
		return s.backupAndUpload(ctx, filePath, objectName)
	}

	// Compare modification times
	localTime := info.ModTime().UTC()
	cloudTime := cloudInfo.ModTime
	diff := cloudTime.Sub(localTime)

	if diff > s.timeTolerance {
		// Cloud is newer, download it
		log.Printf("Cloud file %s is newer (cloud: %v, local: %v), downloading...", 
			objectName, cloudTime, localTime)
		return s.downloadAndReplace(ctx, objectName, filePath, cloudTime)
	} else if diff < -s.timeTolerance {
		// Local is newer, upload it
		log.Printf("Local file %s is newer (cloud: %v, local: %v), uploading...", 
			objectName, cloudTime, localTime)
		return s.backupAndUpload(ctx, filePath, objectName)
	}

	// Files are in sync
	return nil
}

func (s *Syncer) uploadLocalFiles(ctx context.Context) error {
	entries, err := os.ReadDir(s.watchPath)
	if err != nil {
		return fmt.Errorf("failed to read watch directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(s.watchPath, entry.Name())
		if !shouldSyncFile(path) {
			continue
		}

		if err := s.SyncFile(ctx, path); err != nil {
			log.Printf("Failed to sync file %s: %v", path, err)
		}
	}

	return nil
}

func (s *Syncer) downloadCloudFiles(ctx context.Context) error {
	cloudFiles, err := s.storage.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list cloud files: %w", err)
	}

	for _, cloudFile := range cloudFiles {
		if !shouldSyncFile(cloudFile.Name) {
			continue
		}

		localPath := filepath.Join(s.watchPath, filepath.Base(cloudFile.Name))
		localInfo, err := os.Stat(localPath)

		if os.IsNotExist(err) {
			// File doesn't exist locally, download it
			log.Printf("Downloading new file from cloud: %s", cloudFile.Name)
			if err := s.downloadAndReplace(ctx, cloudFile.Name, localPath, cloudFile.ModTime); err != nil {
				log.Printf("Failed to download %s: %v", cloudFile.Name, err)
			}
			continue
		}

		if err != nil {
			log.Printf("Failed to stat local file %s: %v", localPath, err)
			continue
		}

		// Check if cloud is newer
		if cloudFile.ModTime.Sub(localInfo.ModTime().UTC()) > s.timeTolerance {
			log.Printf("Cloud file %s is newer, downloading...", cloudFile.Name)
			if err := s.downloadAndReplace(ctx, cloudFile.Name, localPath, cloudFile.ModTime); err != nil {
				log.Printf("Failed to download %s: %v", cloudFile.Name, err)
			}
		}
	}

	return nil
}

func (s *Syncer) backupAndUpload(ctx context.Context, filePath, objectName string) error {
	// Create backup if file exists
	if fileExists(filePath) {
		if err := s.createBackup(filePath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Upload to cloud
	if err := s.storage.Upload(ctx, filePath, objectName); err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	log.Printf("Uploaded %s to cloud", objectName)
	return nil
}

func (s *Syncer) downloadAndReplace(ctx context.Context, objectName, localPath string, modTime time.Time) error {
	// Create backup if file exists
	if fileExists(localPath) {
		if err := s.createBackup(localPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Download to temp location first
	tempPath := filepath.Join(os.TempDir(), objectName+".download")
	if err := s.storage.Download(ctx, objectName, tempPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Replace local file
	if err := copyFile(tempPath, localPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace local file: %w", err)
	}

	os.Remove(tempPath)

	// Restore modification time
	if err := os.Chtimes(localPath, modTime, modTime); err != nil {
		log.Printf("Warning: failed to set mod time on %s: %v", localPath, err)
	}

	log.Printf("Downloaded and replaced %s", filepath.Base(localPath))
	return nil
}

func (s *Syncer) createBackup(filePath string) error {
	backupPath, err := s.createTimestampedBackupDir()
	if err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupFile := filepath.Join(backupPath, filepath.Base(filePath))
	if err := copyFile(filePath, backupFile); err != nil {
		return fmt.Errorf("failed to copy file to backup: %w", err)
	}

	log.Printf("Created backup: %s", backupFile)
	return nil
}

func (s *Syncer) createTimestampedBackupDir() (string, error) {
	if err := ensureDir(s.backupDir); err != nil {
		return "", err
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05.000000")
	backupPath := filepath.Join(s.backupDir, timestamp)

	if err := os.Mkdir(backupPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create timestamped backup directory: %w", err)
	}

	return backupPath, nil
}

// Utility functions

func shouldSyncFile(filePath string) bool {
	name := filepath.Base(filePath)
	return strings.HasSuffix(name, ".sav") && name != "EnhancedInputUserSettings.sav"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}

	if err := os.WriteFile(dst, input, 0644); err != nil {
		return fmt.Errorf("failed to write destination: %w", err)
	}

	return nil
}

func ensureDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory: %s", path)
	}
	return nil
}
