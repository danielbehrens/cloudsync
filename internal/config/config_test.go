package config

import (
	"os"
	"testing"
)

func TestLoadFromFlags(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Mock command-line arguments
	os.Args = []string{
		"cmd",
		"-watch-path", "/test/path",
		"-process-name", "test.exe",
		"-cloud-endpoint", "test.local:9000",
		"-access-key", "testkey",
		"-secret-key", "testsecret",
		"-bucket-name", "testbucket",
	}

	cfg, err := LoadFromFlags()
	if err != nil {
		t.Fatalf("LoadFromFlags() error = %v", err)
	}

	if cfg.WatchPath != "/test/path" {
		t.Errorf("WatchPath = %v, want %v", cfg.WatchPath, "/test/path")
	}

	if cfg.ProcessName != "test.exe" {
		t.Errorf("ProcessName = %v, want %v", cfg.ProcessName, "test.exe")
	}

	if cfg.S3Config.AccessKey != "testkey" {
		t.Errorf("AccessKey = %v, want %v", cfg.S3Config.AccessKey, "testkey")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "empty watch path",
			cfg: Config{
				WatchPath: "",
			},
			wantErr: true,
		},
		{
			name: "non-existent path",
			cfg: Config{
				WatchPath: "/non/existent/path/12345",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
