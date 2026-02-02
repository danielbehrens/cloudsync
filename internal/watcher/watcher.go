package watcher

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches a directory for file changes
type FileWatcher struct {
	watcher       *fsnotify.Watcher
	watchPath     string
	eventCooldown time.Duration
	lastEventTime map[string]time.Time
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(watchPath string, cooldown time.Duration) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	if err := watcher.Add(watchPath); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch path %s: %w", watchPath, err)
	}

	return &FileWatcher{
		watcher:       watcher,
		watchPath:     watchPath,
		eventCooldown: cooldown,
		lastEventTime: make(map[string]time.Time),
	}, nil
}

// Events returns the channel for file system events
func (fw *FileWatcher) Events() <-chan fsnotify.Event {
	return fw.watcher.Events
}

// Errors returns the channel for watcher errors
func (fw *FileWatcher) Errors() <-chan error {
	return fw.watcher.Errors
}

// Close closes the file watcher
func (fw *FileWatcher) Close() error {
	return fw.watcher.Close()
}

// ShouldProcess determines if an event should be processed based on:
// - File type (must be .sav, excluding EnhancedInputUserSettings.sav)
// - Location (must be in root watch directory)
// - Cooldown period (prevents duplicate events)
func (fw *FileWatcher) ShouldProcess(event fsnotify.Event) bool {
	// Only process write and create events
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	// Check if it's a .sav file (excluding settings)
	if !shouldSyncFile(event.Name) {
		return false
	}

	// Must be in root watch directory (not subdirectories)
	if filepath.Dir(event.Name) != filepath.Clean(fw.watchPath) {
		return false
	}

	// Check cooldown period
	now := time.Now()
	last, seen := fw.lastEventTime[event.Name]
	if seen && now.Sub(last) <= fw.eventCooldown {
		return false
	}

	fw.lastEventTime[event.Name] = now
	return true
}

// shouldSyncFile determines if a file should be synced based on its name
func shouldSyncFile(filePath string) bool {
	name := filepath.Base(filePath)
	return strings.HasSuffix(name, ".sav") && name != "EnhancedInputUserSettings.sav"
}
