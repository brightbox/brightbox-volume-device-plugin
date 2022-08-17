package main

import (
	"sync"

	"github.com/brightbox/brightbox-volume-device-plugin/dpm"
	"github.com/brightbox/brightbox-volume-device-plugin/volwatch"
	"github.com/golang/glog"
	"golang.org/x/exp/maps"
)

// Completion provides a volumes slice and a completion function that needs to
// called when the subscriber plugin has finished with the volumes.
type Completion struct {
	Volumes      []string
	CompleteFunc func()
}

// VolumeLister is a proxy which takes events from the volumewatcher and posts
// them to the plugin manager using the Lister interface
type VolumeLister struct {
	volWatcher *volwatch.VolumeWatcher
	mapmutex   sync.RWMutex
	eventmap   map[string]chan<- Completion
}

// NewLister creates a new volumeLister
func NewLister(vw *volwatch.VolumeWatcher) *VolumeLister {
	return &VolumeLister{
		volWatcher: vw,
		eventmap:   make(map[string]chan<- Completion),
	}
}

// GetResourceNamespace must return namespace (vendor ID) of implemented Lister. e.g. for
// resources in format "color.example.com/<color>" that would be "color.example.com".
func (vl *VolumeLister) GetResourceNamespace() string {
	return resourceNamespace
}

// Discover notifies manager with a list of currently available resources in its namespace.
// e.g. if "color.example.com/red" and "color.example.com/blue" are available in the system,
// it would pass PluginNameList{"red", "blue"} to given channel. In case list of
// resources is static, it would use the channel only once and then return. In case the list is
// dynamic, it could block and pass a new list each times resources changed. If blocking is
// used, it should check whether the channel is closed, i.e. Discover should stop.
func (vl *VolumeLister) Discover(pluginListCh chan dpm.PluginNameList) {
	glog.V(3).Infof("Waiting for volume events\n")
	for {
		select {
		case <-vl.Done():
			glog.V(3).Infof("Exiting Discover: %s\n", vl.volWatcher.Err())
			return
		case event, ok := <-vl.volWatcher.Events():
			if ok {
				glog.V(3).Infoln("Received Watch Event")
				glog.V(3).Infof("Volumes are %v\n", event.Volumes())
				vl.informSubscribers(event.Volumes())
				glog.V(3).Infoln("Notifying manager")
				pluginListCh <- event.Volumes()
			} else {
				glog.V(3).Infoln("Unexpected fault on Watch Event channel")
			}
		}
	}
}

// NewPlugin instantiates a plugin implementation. It is given the last name of the resource,
// e.g. for resource name "color.example.com/red" that would be "red". It must return valid
// implementation of a PluginInterface.
func (vl *VolumeLister) NewPlugin(kind string) dpm.PluginInterface {
	glog.V(3).Infof("Creating device plugin %s", kind)

	return &volumeDevicePlugin{
		kind,
		make(chan Completion),
		vl,
	}
}

// Subscribe adds a channel to the subscription list for volume events
func (vl *VolumeLister) Subscribe(index string, channel chan<- Completion) {
	glog.V(4).Infof("Adding channel subscription for %s", index)
	vl.mapmutex.Lock()
	defer vl.mapmutex.Unlock()
	vl.eventmap[index] = channel
	glog.V(4).Infof("Added")
}

// Unsubscribe removes a channel from the subscription list for volume events
func (vl *VolumeLister) Unsubscribe(index string) {
	glog.V(4).Infof("Removing channel subscription for %s", index)
	vl.mapmutex.Lock()
	defer vl.mapmutex.Unlock()
	delete(vl.eventmap, index)
	glog.V(4).Infof("Removed")
}

// Done returns a channel that is closed when the watcher has been cancelled
func (vl *VolumeLister) Done() <-chan struct{} {
	return vl.volWatcher.Done()
}

// Err returns a Cancelled error when the watcher has been stopped
func (vl *VolumeLister) Err() error {
	return vl.volWatcher.Err()
}

// Implementation

func (vl *VolumeLister) informSubscribers(files []string) {
	glog.V(4).Infoln("Obtaining channels")
	vl.mapmutex.RLock()
	channels := maps.Values(vl.eventmap)
	vl.mapmutex.RUnlock()
	glog.V(4).Infoln("Informing Subscribers")
	var wg sync.WaitGroup
	for _, channel := range channels {
		select {
		case <-vl.volWatcher.Done():
			glog.V(4).Infoln("Watcher is done, shouldn't get here")
		default:
			wg.Add(1)
			channel <- Completion{files, wg.Done}
		}
	}
	glog.V(4).Infoln("Waiting for Subscribers to complete updates")
	wg.Wait()
}

const (
	resourceNamespace = "volumes.brightbox.com"
)
