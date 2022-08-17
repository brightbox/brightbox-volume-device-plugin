package main

import (
	"context"

	"golang.org/x/exp/slices"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type volumeDevicePlugin struct {
	volumeID     string
	volumeUpdate chan Completion
	volLister    *VolumeLister
}

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (vdp *volumeDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	glog.V(3).Info("Volume GetDevicePluginOptions Called")

	return &pluginapi.DevicePluginOptions{}, nil
}

func isRemoved(event fsnotify.Event) bool {
	return event.Op == fsnotify.Remove
}

var volMissing = &pluginapi.ListAndWatchResponse{Devices: []*pluginapi.Device{}}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (vdp *volumeDevicePlugin) ListAndWatch(empty *pluginapi.Empty, srv pluginapi.DevicePlugin_ListAndWatchServer) error {
	glog.V(3).Info("Volume ListAndWatch Called")
	glog.V(3).Infof("Volume %s: Notifying kubelet", vdp.volumeID)
	volPresent := &pluginapi.ListAndWatchResponse{
		Devices: []*pluginapi.Device{
			&pluginapi.Device{
				ID:     vdp.volumeID,
				Health: pluginapi.Healthy,
			},
		},
	}
	if err := srv.Send(volPresent); err != nil {
		glog.V(3).Infof("Volume %s: Failed to send volume present: %s", vdp.volumeID, err)
		return err
	}
	vdp.volLister.Subscribe(vdp.volumeID, vdp.volumeUpdate)
	defer vdp.volLister.Unsubscribe(vdp.volumeID)
	glog.V(3).Infof("Volume %s: Waiting for updates", vdp.volumeID)
	for {
		select {
		case <-vdp.volLister.Done():
			glog.V(3).Infof("Volume %s: Exiting ListAndWatch: %s\n", vdp.volumeID, vdp.volLister.Err())
			err := srv.Send(volMissing)
			if err != nil {
				glog.V(3).Infof("Volume %s: Failed to send volume missing: %s", vdp.volumeID, err)
				return err
			}
			return vdp.volLister.Err()
		case completion, ok := <-vdp.volumeUpdate:
			glog.V(3).Infof("Volume %s: Received update", vdp.volumeID)
			if !(ok && slices.Contains(completion.Volumes, vdp.volumeID)) {
				glog.V(3).Infof("Volume %s: missing from list, updating and exiting", vdp.volumeID)
				err := srv.Send(volMissing)
				completion.CompleteFunc()
				if err != nil {
					glog.V(3).Infof("Volume %s: Failed to send volume missing: %s", vdp.volumeID, err)
					return err
				}
				return nil
			}
			completion.CompleteFunc()
			glog.V(3).Infof("Volume %s: still in list", vdp.volumeID)
			glog.V(3).Infof("Volume %s: Waiting for updates", vdp.volumeID)
		}
	}
}

// GetPreferredAllocation returns a preferred set of devices to allocate
// from a list of available ones. The resulting preferred allocation is not
// guaranteed to be the allocation ultimately performed by the
// devicemanager. It is only designed to help the devicemanager make a more
// informed allocation decision when possible.
func (vdp *volumeDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return nil, nil
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (vdp *volumeDevicePlugin) Allocate(ctx context.Context, request *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	glog.V(3).Info("Volume Allocate Called")
	glog.V(4).Infof("Request is %#v", request)
	return &pluginapi.AllocateResponse{
		ContainerResponses: []*pluginapi.ContainerAllocateResponse{
			&pluginapi.ContainerAllocateResponse{},
		},
	}, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase,
// before each container start. Device plugin can run device specific operations
// such as resetting the device before making devices available to the container
func (vdp *volumeDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return nil, nil
}
