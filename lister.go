package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
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
			if waitForChanges(path.Dir(watchDir), isWatchDir) {
				glog.V(3).Infoln("Directory watch complete - rediscovering")
			} else {
				glog.Warning("Directory watch failure - rediscovering")
			}
		} else {
			glog.V(3).Infof("Enumerating volumes at %s", watchDir)
			pluginListCh <- enumerateVolumes(files)
			if waitForChanges(watchDir, isVolume) {
				glog.V(3).Infoln("Volume watch complete - rediscovering")
			} else {
				glog.Warning("Volume watch failure - rediscovering")
			}
		}
	}
}

func (scl volumeLister) NewPlugin(kind string) dpm.PluginInterface {
	glog.V(3).Infof("Creating device plugin %s", kind)

	return &volumeDevicePlugin{kind}
}

func fatal(err error) {
	if err == nil {
		return
	}
	glog.Errorf("%s: %s\n", filepath.Base(os.Args[0]), err)
	os.Exit(1)
}

func isWatchDir(event fsnotify.Event) bool {
	return event.Op == fsnotify.Create &&
		event.Name == watchDir
}

func isVolume(event fsnotify.Event) bool {
	return (event.Op == fsnotify.Create ||
		event.Op == fsnotify.Remove) && path.Dir(event.Name) == watchDir
}

func waitForChanges(dirName string, matchEvent func(fsnotify.Event) bool) bool {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal(err)
	}
	defer watcher.Close()
	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					glog.V(3).Infoln("Watch event issue: exiting")
					done <- false
					return
				}
				if matchEvent(event) {
					glog.V(3).Infof("Watch event: %q %s\n", event.Name, event.Op)
					done <- true
					return
				}
				glog.V(3).Infoln("Ignored watch event:", event)
			case err, ok := <-watcher.Errors:
				if !ok {
					glog.V(3).Infoln("Watch error issue: exiting")
					done <- false
					return
				}
				glog.V(3).Infoln("Watch error:", err)
				done <- false
				return
			}
		}
	}()
	glog.V(3).Infof("Adding watch for %q\n", dirName)
	err = watcher.Add(dirName)
	if err != nil {
		fatal(fmt.Errorf("%q: %w", dirName, err))
	}
	return <-done
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
