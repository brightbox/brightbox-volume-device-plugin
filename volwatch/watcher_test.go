package volwatch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchCancel(t *testing.T) {
	baseDir := t.TempDir()
	watchDir := filepath.Join(baseDir, "by-id")
	watch := NewWatchDir(watchDir)
	watch.Cancel()
	select {
	case <-watch.Done():
	default:
		t.Error("Watch failed to Cancel properly")
	}
}

func TestWatchCreate(t *testing.T) {
	baseDir := t.TempDir()
	watchDir := filepath.Join(baseDir, "by-id")
	watch := NewWatchDir(watchDir)
	defer watch.Cancel()
	os.Mkdir(watchDir, 0755)
	select {
	case event, ok := <-watch.Events():
		if !ok {
			t.Error("Failed to enumerate directories")
		}
		if len(event) != 0 {
			t.Errorf("Expected no directory entries, got %d", len(event))
		}
	}
}
