package main

import (
	"flag"

	"github.com/brightbox/brightbox-volume-device-plugin/dpm"
	"github.com/brightbox/brightbox-volume-device-plugin/volwatch"
)

func main() {
	flag.Parse()

	// Kubernetes plugin uses the kubernetes library, which uses glog, which logs to the filesystem by default,
	// while we need all logs to go to stderr
	// See also: https://github.com/coredns/coredns/pull/1598
	flag.Set("logtostderr", "true")

	// manager := dpm.NewManager(volumeLister{})
	// manager.Run()
	watcher := volwatch.NewWatcher()
	lister := NewLister(watcher)
	manager := dpm.NewManager(lister)
	manager.Run()
}
