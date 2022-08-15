package volwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"golang.org/x/exp/maps"
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
	mapmutex sync.RWMutex
	eventmap map[string]chan<- Event
	events   chan Event
	ctx      context.Context
	cancel   context.CancelFunc
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
		eventmap: make(map[string]chan<- Event),
		ctx:      watchCtx,
		cancel:   watchCancel,
	}
	go watcher.run()
	return watcher
}

// Subscribe adds a channel to the subscription list for volume events
func (vw *VolumeWatcher) Subscribe(index string, channel chan<- Event) {
	glog.V(4).Infof("Adding channel subscription for %s", index)
	vw.mapmutex.Lock()
	defer vw.mapmutex.Unlock()
	vw.eventmap[index] = channel
	glog.V(4).Infof("Added")
}

// Unsubscribe removes a channel from the subscription list for volume events
func (vw *VolumeWatcher) Unsubscribe(index string) {
	glog.V(4).Infof("Removing channel subscription for %s", index)
	vw.mapmutex.Lock()
	defer vw.mapmutex.Unlock()
	delete(vw.eventmap, index)
	glog.V(4).Infof("Removed")
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
			volumes := enumerateVolumes(files)
			vw.informSubscribers(volumes)
			vw.events <- volumes
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

func (vw *VolumeWatcher) informSubscribers(files Event) {
	glog.V(4).Infoln("Informing Subscribers")
	glog.V(4).Infoln("Obtaining channels")
	vw.mapmutex.RLock()
	channels := maps.Values(vw.eventmap)
	vw.mapmutex.RUnlock()
	glog.V(4).Infoln("Sending channel updates")
	for _, channel := range channels {
		select {
		case <-vw.ctx.Done():
			glog.V(4).Infoln("Watcher is done, shouldn't get here")
		default:
			channel <- files
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
