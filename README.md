## Brightbox Block Storage Volume device plugin for Kubernetes

The repository builds a container image which is stored on Docker Hub.

The plugin can be installed via the `daemonset.yaml` manifest.

```
kubectl apply -f https://raw.githubusercontent.com/brightbox/brightbox-volume-device-plugin/main/daemonset.yaml
```

## What it does
The plugin watches for volume attach and detach events on Brightbox servers and creates a custom resource within Kubernetes.
The resources are of the form

```
volumes.brightbox.com/vol-tgl4c
```

The list of volume resources can be viewed via `kubectl describe node`

Pods can require a particular volume to be present by adding the volume to their resources stanzas

```
apiVersion: v1
kind: Pod
metadata:
  name: with-volume-requirement
spec:
  containers:
  - name: with-volume-requirement
    image: k8s.gcr.io/pause:2.0
    resources:
      limits:
        volumes.brightbox.com/vol-qsk4v: 1
```
