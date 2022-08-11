package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"

	"github.com/brightbox/brightbox-volume-device-plugin/dpm"
	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
)

type volumeLister struct {
}

const resourceNamespace = "volumes.brightbox.com"
const watchDir = "/dev/disk/by-id"

var volRe = regexp.MustCompile(`vol-.....$`)

func (scl volumeLister) GetResourceNamespace() string {
	return resourceNamespace
}

func (scl volumeLister) Discover(pluginListCh chan dpm.PluginNameList) {
	for {
		files, err := os.ReadDir(watchDir)
		if errors.Is(err, os.ErrNotExist) {
			glog.Warningf("Watch directory %s is missing", watchDir)
			pluginListCh <- nil
			if err := waitForChanges(path.Dir(watchDir), isWatchDir); err != nil {
				glog.Warningf("Directory watch failure: %+v\n", err)
			}
			glog.V(3).Infoln("Directory watch complete - rediscovering")
		} else {
			glog.V(3).Infof("Enumerating volumes at %s", watchDir)
			pluginListCh <- enumerateVolumes(files)
			if err := waitForChanges(watchDir, isVolume); err != nil {
				glog.Warningf("Volume watch failure %+v\n", err)
			}
			glog.V(3).Infoln("Volume watch complete - rediscovering")
		}
	}
}

func (scl volumeLister) NewPlugin(kind string) dpm.PluginInterface {
	glog.V(3).Infof("Creating device plugin %s", kind)

	return &volumeDevicePlugin{kind}
}

func isWatchDir(event fsnotify.Event) bool {
	return event.Op == fsnotify.Create &&
		event.Name == watchDir
}

func isVolume(event fsnotify.Event) bool {
	return (event.Op == fsnotify.Create ||
		event.Op == fsnotify.Remove) && path.Dir(event.Name) == watchDir
}

func waitForChanges(dirName string, matchEvent func(fsnotify.Event) bool) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("Failed to create watcher for %s: %w", dirName, err)
	}
	defer watcher.Close()
	glog.V(3).Infof("Adding watch for %q\n", dirName)
	err = watcher.Add(dirName)
	if err != nil {
		return fmt.Errorf("Failed to add %s to watcher: %w", dirName, err)
	}
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("Watch event issue")
			}
			if matchEvent(event) {
				glog.V(3).Infof("Watch event: %q %s\n", event.Name, event.Op)
				return nil
			}
			glog.V(3).Infoln("Ignored watch event:", event)
		case err := <-watcher.Errors:
			return err
		}
	}
}

func enumerateVolumes(dirents []os.DirEntry) []string {
	result := make([]string, 0, len(dirents))
	for _, ent := range dirents {
		if ent.IsDir() {
			continue
		}
		if m := volRe.FindString(ent.Name()); m != "" {
			result = append(result, m)
		}
	}
	return result
}
