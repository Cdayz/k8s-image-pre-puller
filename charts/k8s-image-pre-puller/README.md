# k8s-image-pre-puller

A Helm chart for k8s-image-pre-puller operator

## Introduction

This chart bootstraps a [Kubernetes Operator for Prepulling Docker Images](https://github.com/Cdayz/k8s-image-pre-puller) deployment using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Helm >= 3
- Kubernetes >= 1.16

## Installing the chart

```shell
helm repo add k8s-image-pre-puller https://cdayz.github.io/k8s-image-pre-puller
helm install my-release k8s-image-pre-puller/k8s-image-pre-puller
```

This will create a release of `k8s-image-pre-puller` in the default namespace. To install in a different one:

```shell
helm install -n k8s-image-pre-puller-ns my-release k8s-image-pre-puller/k8s-image-pre-puller
```

Note that `helm` will fail to install if the namespace doesn't exist. Either create the namespace beforehand or pass the `--create-namespace` flag to the `helm install` command.

## Uninstalling the chart

To uninstall `my-release`:

```shell
helm uninstall my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release, except for the `crds`, those will have to be removed manually.

## Test the chart

Install [chart-testing cli](https://github.com/helm/chart-testing#installation)

In Mac OS, you can just:

```bash
pip install yamale
pip install yamllint
brew install chart-testing
```

Run ct lint and Verify `All charts linted successfully`

```bash
Linting chart "k8s-image-pre-puller => (version: \"1.1.28\", path: \"charts/k8s-image-pre-puller\")"
Validating /Users/cdayz/Sources/opensource/Cdayz/k8s-image-pre-puller/charts/k8s-image-pre-puller/Chart.yaml...
Validation success! 👍
Validating maintainers...
==> Linting charts/k8s-image-pre-puller
[INFO] Chart.yaml: icon is recommended

1 chart(s) linted, 0 chart(s) failed

------------------------------------------------------------------------------------------------------------------------
 ✔︎ k8s-image-pre-puller => (version: "1.1.28", path: "charts/k8s-image-pre-puller")
------------------------------------------------------------------------------------------------------------------------
All charts linted successfully
```
