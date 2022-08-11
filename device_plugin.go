package main

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type volumeDevicePlugin struct {
	volumeID string
}

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (dpi *volumeDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	glog.V(3).Info("Volume GetDevicePluginOptions Called")

	return &pluginapi.DevicePluginOptions{}, nil
}

func isRemoved(event fsnotify.Event) bool {
	return event.Op == fsnotify.Remove
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (dpi *volumeDevicePlugin) ListAndWatch(empty *pluginapi.Empty, srv pluginapi.DevicePlugin_ListAndWatchServer) error {
	glog.V(3).Info("Volume ListAndWatch Called")
	glog.V(3).Infof("Volume %s: Notifying kublet", dpi.volumeID)
	if err := srv.Send(&pluginapi.ListAndWatchResponse{
		Devices: []*pluginapi.Device{
			&pluginapi.Device{
				ID:     dpi.volumeID,
				Health: pluginapi.Healthy,
			},
		},
	}); err != nil {
		return err
	}
	glog.V(3).Infof("Volume %s: Blocking Watch", dpi.volumeID)
	<-make(chan bool)
	return nil
}

// GetPreferredAllocation returns a preferred set of devices to allocate
// from a list of available ones. The resulting preferred allocation is not
// guaranteed to be the allocation ultimately performed by the
// devicemanager. It is only designed to help the devicemanager make a more
// informed allocation decision when possible.
func (dpi *volumeDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return nil, nil
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (dpi *volumeDevicePlugin) Allocate(context.Context, *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	glog.V(3).Info("Volume Allocate Called")
	return nil, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase,
// before each container start. Device plugin can run device specific operations
// such as resetting the device before making devices available to the container
func (dpi *volumeDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return nil, nil
}
