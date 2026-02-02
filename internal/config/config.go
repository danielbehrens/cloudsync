package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all application configuration
type Config struct {
	WatchPath   string
	ProcessName string
	BackupDir   string
	S3Config    S3Config
}

// S3Config holds S3/MinIO connection details
type S3Config struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	BucketName string
	UseSSL     bool
}

// LoadFromFlags parses command-line flags and returns a Config
func LoadFromFlags() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.WatchPath, "watch-path", "", "Path to watch for file changes (auto-generated if empty)")
	flag.StringVar(&cfg.ProcessName, "process-name", "RSDragonwilds-Win64-Shipping.exe", "Process name to pause sync when running")
	flag.StringVar(&cfg.BackupDir, "backup-dir", "", "Directory to store backups (auto-generated if empty)")
	flag.StringVar(&cfg.S3Config.Endpoint, "cloud-endpoint", "localhost:9000", "MinIO/S3 cloud endpoint")
	flag.StringVar(&cfg.S3Config.AccessKey, "access-key", "", "Cloud storage access key")
	flag.StringVar(&cfg.S3Config.SecretKey, "secret-key", "", "Cloud storage secret key")
	flag.StringVar(&cfg.S3Config.BucketName, "bucket-name", "gamesync-dragonwilds", "Bucket name in cloud storage")

	flag.Parse()

	// Validate required fields
	if cfg.S3Config.Endpoint == "" || cfg.S3Config.AccessKey == "" || 
	   cfg.S3Config.SecretKey == "" || cfg.S3Config.BucketName == "" {
		return nil, fmt.Errorf("missing required arguments: cloud-endpoint, access-key, secret-key, or bucket-name")
	}

	// Determine SSL from endpoint
	cfg.S3Config.UseSSL = strings.HasPrefix(cfg.S3Config.Endpoint, "https://")

	// Auto-generate watchPath if not provided
	if cfg.WatchPath == "" {
		var err error
		cfg.WatchPath, err = getDefaultWatchPath()
		if err != nil {
			return nil, fmt.Errorf("failed to determine default watch path: %w", err)
		}
	}

	// Auto-generate backupDir if not provided
	if cfg.BackupDir == "" {
		cfg.BackupDir = filepath.Join(cfg.WatchPath, "Backup")
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.WatchPath == "" {
		return fmt.Errorf("watch path cannot be empty")
	}

	info, err := os.Stat(c.WatchPath)
	if err != nil {
		return fmt.Errorf("watch path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("watch path is not a directory: %s", c.WatchPath)
	}

	return nil
}

func getDefaultWatchPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("LOCALAPPDATA environment variable is not set")
	}
	return filepath.Join(localAppData, "RSDragonwilds", "Saved", "SaveGames"), nil
}
