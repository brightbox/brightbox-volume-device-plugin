package volwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
)

const (
	// Create is a volume creation event
	Create = iota
	// Remove is a volume deletion event
	Remove
)

// Event is returned by the events channel
type Event []string

// Volumes extracts the list of volumes from an Event type
func (e Event) Volumes() []string {
	return []string(e)
}

// VolumeWatcher watches the disk area for new volumes
// and posts them to the Events channel
//
// Create a VolumeWatcher by calling the NewWatcher function
type VolumeWatcher struct {
	events chan Event
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWatcher creates a new volume watcher.
// It launches a separate Go routine in a separate context which
// watches for volumes being created and removed.
// The watcher can be cancelled by calling the returned context cancellation
// function
func NewWatcher() *VolumeWatcher {
	glog.V(4).Infof("Creating new watcher")

	watchCtx, watchCancel := context.WithCancel(context.Background())
	watcher := &VolumeWatcher{
		events: make(chan Event),
		ctx:    watchCtx,
		cancel: watchCancel,
	}
	go watcher.run()
	return watcher
}

// Events returns the main events channel
func (vw *VolumeWatcher) Events() <-chan Event {
	return vw.events
}

// Done returns a channel that is closed when the watcher has been cancelled
func (vw *VolumeWatcher) Done() <-chan struct{} {
	return vw.ctx.Done()
}

// Cancel signals to the watcher that it should stop watching and close down
func (vw *VolumeWatcher) Cancel() {
	vw.cancel()
}

// Err returns a Cancelled error when the watcher has been stopped
func (vw *VolumeWatcher) Err() error {
	return vw.ctx.Err()
}

// Implementation

const watchDir = "/dev/disk/by-id"

var volRe = regexp.MustCompile(`vol-.....$`)

// run sets up the watcher and reports events
// Runs until cancelled via the supplied context
func (vw *VolumeWatcher) run() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		glog.Warningf("Unable to create Watcher")
		return
	}
	defer watcher.Close()
	for {
		files, err := os.ReadDir(watchDir)
		if errors.Is(err, os.ErrNotExist) {
			glog.Warningf("Watch directory %s is missing", watchDir)
			if err := waitForChanges(vw.ctx, watcher, path.Dir(watchDir), isWatchDir); err != nil {
				glog.Warningf("Directory watch failure: %+v\n", err)
			}
		} else {
			glog.V(4).Infof("Enumerating volumes at %s", watchDir)
			glog.V(4).Infoln("Notifying volume lister")
			vw.events <- enumerateVolumes(files)
			if err := waitForChanges(vw.ctx, watcher, watchDir, isVolume); err != nil {
				glog.Warningf("Volume watch failure %+v\n", err)
			}
		}
		select {
		case <-vw.ctx.Done():
			glog.V(4).Infoln("Directory scanner cancelled")
			return
		default:
			glog.V(4).Infoln("Directory watch complete - rediscovering")
		}
	}
}

func isWatchDir(event fsnotify.Event) bool {
	return event.Has(fsnotify.Create) &&
		event.Name == watchDir
}

func isVolume(event fsnotify.Event) bool {
	return (event.Has(fsnotify.Create) ||
		event.Has(fsnotify.Remove)) && path.Dir(event.Name) == watchDir
}

func waitForChanges(ctx context.Context, watcher *fsnotify.Watcher, dirName string, matchEvent func(fsnotify.Event) bool) error {
	err := watcher.Add(dirName)
	if err != nil {
		return fmt.Errorf("Failed to add %s to watcher: %w", dirName, err)
	}
	defer watcher.Remove(dirName)
	glog.V(4).Infoln("Waiting for device changes")
	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infoln("Watch cancelled")
			return ctx.Err()
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("Watch event issue")
			}
			if matchEvent(event) {
				glog.V(4).Infof("Watch event: %q %s\n", event.Name, event.Op)
				return nil
			}
			glog.V(4).Infoln("Ignored watch event:", event)
		case err := <-watcher.Errors:
			return err
		}
	}
}

func enumerateVolumes(dirents []os.DirEntry) Event {
	result := make([]string, 0, len(dirents))
	for _, ent := range dirents {
		if ent.IsDir() {
			continue
		}
		if m := volRe.FindString(ent.Name()); m != "" {
			result = append(result, m)
		}
	}
	return Event(result)
}
