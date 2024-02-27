# K8S Image Pre Puller

![CI](https://github.com/Cdayz/k8s-image-pre-puller/actions/workflows/main.yml/badge.svg)

It's a simple kubernetes operator that will help you automatically pre-pull docker images on nodes.
This operator can be used for speed-up pod initialization in cluster.

## How to use

### Install

// TODO: Install instructions

### Create PrePullImage

When operator installed in cluster you can simply create resources like this:

```yaml
apiVersion: v1
kind: PrePullImage
metadata:
  name: my-cool-image-pre-pull
  namespace: my-namespace
spec:
  image: "gcr.io/my-cool-image"
  nodeSelector: {} # You can specify selectors for pre-pull only on particular nodes
```

This operator automatically will create a `DaemonSet` for create pre-pulling pods on every existing node.

**IMPORTANT**: This operator creates a `DaemonSet` and it will create `Pod`'s in cluster - this pods will consume resources according to configuration. It's important to have monitoring about this resources.
