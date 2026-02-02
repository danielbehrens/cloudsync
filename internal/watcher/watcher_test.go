package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestShouldSyncFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "valid sav file",
			filePath: "/path/to/game.sav",
			want:     true,
		},
		{
			name:     "excluded settings file",
			filePath: "/path/to/EnhancedInputUserSettings.sav",
			want:     false,
		},
		{
			name:     "non-sav file",
			filePath: "/path/to/game.txt",
			want:     false,
		},
		{
			name:     "sav extension check",
			filePath: "/path/to/savegame.sav",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncFile(tt.filePath); got != tt.want {
				t.Errorf("shouldSyncFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFileWatcher(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	fw, err := NewFileWatcher(tmpDir, time.Second)
	if err != nil {
		t.Fatalf("NewFileWatcher() error = %v", err)
	}
	defer fw.Close()

	if fw.watchPath != tmpDir {
		t.Errorf("watchPath = %v, want %v", fw.watchPath, tmpDir)
	}

	if fw.eventCooldown != time.Second {
		t.Errorf("eventCooldown = %v, want %v", fw.eventCooldown, time.Second)
	}
}

func TestFileWatcherShouldProcess(t *testing.T) {
	tmpDir := t.TempDir()

	fw, err := NewFileWatcher(tmpDir, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileWatcher() error = %v", err)
	}
	defer fw.Close()

	testFile := filepath.Join(tmpDir, "test.sav")

	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{
			name: "write event for sav file",
			event: fsnotify.Event{
				Name: testFile,
				Op:   fsnotify.Write,
			},
			want: true,
		},
		{
			name: "create event for sav file",
			event: fsnotify.Event{
				Name: testFile,
				Op:   fsnotify.Create,
			},
			want: true,
		},
		{
			name: "remove event (should be ignored)",
			event: fsnotify.Event{
				Name: testFile,
				Op:   fsnotify.Remove,
			},
			want: false,
		},
		{
			name: "excluded settings file",
			event: fsnotify.Event{
				Name: filepath.Join(tmpDir, "EnhancedInputUserSettings.sav"),
				Op:   fsnotify.Write,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cooldown state
			delete(fw.lastEventTime, tt.event.Name)

			if got := fw.ShouldProcess(tt.event); got != tt.want {
				t.Errorf("ShouldProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFileWatcherCooldown(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.sav")

	fw, err := NewFileWatcher(tmpDir, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileWatcher() error = %v", err)
	}
	defer fw.Close()

	event := fsnotify.Event{
		Name: testFile,
		Op:   fsnotify.Write,
	}

	// First event should be processed
	if !fw.ShouldProcess(event) {
		t.Error("First event should be processed")
	}

	// Second immediate event should be blocked by cooldown
	if fw.ShouldProcess(event) {
		t.Error("Second immediate event should be blocked by cooldown")
	}

	// Wait for cooldown period
	time.Sleep(250 * time.Millisecond)

	// Event after cooldown should be processed
	if !fw.ShouldProcess(event) {
		t.Error("Event after cooldown should be processed")
	}
}
