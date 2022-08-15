package main

import (
	"github.com/brightbox/brightbox-volume-device-plugin/dpm"
	"github.com/brightbox/brightbox-volume-device-plugin/volwatch"
	"github.com/golang/glog"
)

// VolumeLister is a proxy which takes events from the volumewatcher and posts
// them to the plugin manager using the Lister interface
type VolumeLister struct {
	volWatcher *volwatch.VolumeWatcher
}

// NewLister creates a new volumeLister
func NewLister(vw *volwatch.VolumeWatcher) *VolumeLister {
	return &VolumeLister{vw}
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
		case <-vl.volWatcher.Done():
			glog.V(3).Infof("Exiting Discover: %s\n", vl.volWatcher.Err())
			return
		case event, ok := <-vl.volWatcher.Events():
			if ok {
				glog.V(3).Infoln("Received Watch Event")
				glog.V(3).Infoln("Notifying manager")
				glog.V(3).Infof("Volumes are %v\n", event.Volumes())
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
		make(chan volwatch.Event),
		vl.volWatcher,
	}
}

// Implementation

const (
	resourceNamespace = "volumes.brightbox.com"
	subscriptionName  = "lister"
)
