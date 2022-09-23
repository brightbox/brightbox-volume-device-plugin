package volwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
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
	watch  *fsnotify.Watcher
}

// IDDevicePath gives the full path to the target in the deviceDir
func IDDevicePath(target string) string {
	return filepath.Join(deviceDir, target)
}

// NewWatcher creates a new volume watcher.
// It launches a separate Go routine in a separate context which
// watches for volumes being created and removed.
// The watcher can be cancelled by calling the returned context cancellation
// function
func NewWatcher() *VolumeWatcher {
	return NewWatchDir(deviceDir)
}

// NewWatchDir creates a new volume watcher on an arbitrary directory
func NewWatchDir(dir string) *VolumeWatcher {
	glog.V(4).Infof("Creating new watcher")

	watch, err := fsnotify.NewWatcher()
	if err != nil {
		glog.Warningf("Unable to create file Watcher")
		return nil
	}
	watchCtx, watchCancel := context.WithCancel(context.Background())
	watcher := &VolumeWatcher{
		events: make(chan Event),
		ctx:    watchCtx,
		cancel: watchCancel,
		watch:  watch,
	}
	go watcher.run(dir)
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

const deviceDir = "/dev/disk/by-id"
const bufferSize = 3

var volRe = regexp.MustCompile(`vol-.....$`)

// run sets up the watcher and reports events
// Runs until cancelled via the supplied context
func (vw *VolumeWatcher) run(watchDir string) {
	baseDir := path.Dir(watchDir)
	defer vw.watch.Close()
	if err := vw.watch.Add(baseDir); err != nil {
		vw.warnAndCancel(
			fmt.Sprintf("Failed to add %s to watcher", baseDir),
			err,
		)
		return
	}
	if err := vw.watch.Add(watchDir); err == nil {
		vw.readAndNotify(watchDir)
	} else {
		glog.Infoln("Watch Directory is missing - awaiting create")
	}
	for {
		select {
		case err := <-vw.watch.Errors:
			vw.warnAndCancel("Unexpected volume watch errors", err)
		case <-vw.ctx.Done():
			glog.V(4).Infoln("Directory scanner cancelled")
			return
		case event, ok := <-vw.watch.Events:
			switch {
			case !ok:
				vw.warnAndCancel(
					"Unexpected volume watch event error",
					fmt.Errorf("watch event error"),
				)
			case isDirRemove(event, watchDir):
				glog.V(4).Infoln("Watch Directory removed", event)
			case isDirRemove(event, baseDir):
				glog.V(4).Infoln("Base Directory removed", event)
				glog.Warning("Cancelling watch")
				vw.cancel()
			case isDirCreate(event, watchDir):
				glog.V(4).Infoln("Watch Directory added")
				if err := vw.watch.Add(watchDir); err == nil {
					vw.readAndNotify(watchDir)
				} else {
					vw.warnAndCancel(
						fmt.Sprintf("Failed to add %s to watcher", watchDir),
						err,
					)
				}
			case isVolChange(event, watchDir):
				glog.V(4).Infoln("Watch Directory changed", event)
				vw.readAndNotify(watchDir)
			default:
				glog.V(4).Infoln("Ignored watch event: ", event)
			}
		}
	}
}

func (vw *VolumeWatcher) warnAndCancel(message string, err error) {
	glog.Warningf("%s: %s", message, err)
	glog.Warning("Cancelling watch")
	vw.cancel()
}

func (vw *VolumeWatcher) readAndNotify(watchDir string) {
	files, err := os.ReadDir(watchDir)
	if err == nil {
		glog.V(4).Infof("Enumerating volumes at %s\n", watchDir)
		glog.V(4).Infoln("Adding event to lister queue")
		vw.events <- enumerateVolumes(files)
	} else if errors.Is(err, os.ErrNotExist) {
		glog.V(4).Infoln("Watch Directory removed during event")
	} else {
		vw.warnAndCancel(
			fmt.Sprintf("Failed to read %s", watchDir),
			err,
		)
	}
}

func isDirRemove(event fsnotify.Event, targetDir string) bool {
	return event.Has(fsnotify.Remove) &&
		event.Name == targetDir
}

func isDirCreate(event fsnotify.Event, targetDir string) bool {
	return event.Has(fsnotify.Create) &&
		event.Name == targetDir
}

func isVolChange(event fsnotify.Event, targetDir string) bool {
	return (event.Has(fsnotify.Create) ||
		event.Has(fsnotify.Remove)) && path.Dir(event.Name) == targetDir
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
