[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/couchbaselabs/couchbase-reschedule-hook)](https://goreportcard.com/report/github.com/couchbaselabs/couchbase-reschedule-hook)

# Couchbase Reschedule Hook

The Couchbase Reschedule Hook is an open source project designed to help with the graceful handling of eviction requests for operator-managed Kubernetes (K8s) pods. Once running and configured using a controller and a validating webhook, eviction requests for select pods will be rejected, with an annotation added to those pods so that an operator can safely reschedule them. The project is intended specifically to protect stateful applications from K8s [node drains](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/), whereby kubectl will cordon a node and evict all pods that reside on it.

## Overview

The reschedule hook was initially conceived to work alongside the [Couchbase Autonomous Operator](https://www.couchbase.com/products/operator/) (CAO), but there are a number of environment variables that can be [configured](#configuration) to enable it to work with other operator managed applications. In the 2.8.0 CAO release, a [reschedule annotation](https://docs.couchbase.com/operator/current/reference-annotations.html#pod-rescheduling) was implemented, allowing cluster administrators to manually mark operator-managed pods for rescheduling. Using this annotation, the reschedule hook will reject eviction requests while marking pods for rescheduling, which, in the case of node drains, means pods will be recreated on uncordoned nodes.

In scenarios where pods will be rescheduled with the same name, the reschedule hook can use another K8s resource to track which pods have already been marked for rescheduling, which is required due to how the drain command works internally. By default, the pod's associated Couchbase Cluster will be used, but this can be changed or disabled entirely.

See [LINK TO BLOG](blog) for a more detailed look into why this alternative approach to node drains is useful when using K8s to run stateful applications like Couchbase and how the project works alongside a CAO managed couchbase cluster.

## Prerequisites

The following tools are required to work with the codebase:

- [git](https://git-scm.com/)
- [go](https://go.dev/)
- [docker](https://www.docker.com/)
- [GNU Make](https://www.gnu.org/software/make/manual/make.html)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

Recommended development tools:
- [kind](https://kubernetes.io/docs/tasks/tools/) - For running K8s locally
- [act](https://github.com/nektos/act) - For running GitHub Actions locally
- [golangci-lint](https://golangci-lint.run/) - For code linting

## Deployment

While this project is not currently available as a public image, a number of files and commands have been added to help with deploying the Couchbase Reschedule Hook into a K8s cluster. The following is a basic step-by-step guide to get the reschedule hook up and running in an existing cluster.

### Clone The Repository
```bash
git clone https://github.com/couchbaselabs/couchbase-reschedule-hook.git
cd couchbase-reschedule-hook
```

### Build The Image

There are a number of [makefile](Makefile) commands to help build and optionally push it to a repository where it can be used to create containers.

For local development, build and load the image into a Kind cluster:
```bash
make kind-image KIND_CLUSTER_NAME=<mycluster> DOCKER_USER=<myuser> DOCKER_IMAGE=<myimage> DOCKER_TAG=<mytag>
```

To build and push the image to a public docker repository:
```bash
make public-image DOCKER_USER=<myuser> DOCKER_IMAGE=<myimage> DOCKER_TAG=<mytag>
```

To just build the image:
```bash
make docker-image DOCKER_USER=<myuser> DOCKER_IMAGE=<myimage> DOCKER_TAG=<mytag>
```

### Generate TLS (Optional)

By default, validating webhooks in K8s require TLS as API server calls are done using HTTPS. Webhook servers must present valid TLS certificates signed by a trusted CA. When the webhook is created, the CA bundle must be provided in the `caBundle` field of the `ValidatingWebhookConfiguration` manifest. For development, a valid TLS secret and caBundle can be generated:
```bash
./scripts/generate-certs.sh
```

This creates a valid `kubernetes.io/tls` secret named `reschedule-hook-tls` and updates the required field in the example `validating-webhook-config.yaml` manifest. In production, this should be configured and managed by a cluster administrator.

### Deploy the Kubernetes Resources

In the `./deploy` directory, there are two files to help deploy the reschedule hook into an existing K8s cluster.

The [reschedule hook deployment](/deploy/reschedule-hook-deployment.yaml) file contains all the required K8s resources to setup the reschedule hook stack:
* Service
* ServiceAccount
* ClusterRole
* ClusterRoleBinding
* Deployment

The pod spec in the `Deployment` manifest will need to be updated to allow the container to pull and run the reschedule hook image. The environment variables should also be [configured](#configuration) for your required setup. Deploy the stack with:

```bash
kubectl apply -f deploy/reschedule-hook-deployment.yaml
```

The [webhook config](/deploy/validating-webhook-config.yaml) file contains the manifest for the webhook which redirects eviction requests to the reschedule hook service. If `generate-certs.sh` was used to generate a caBundle, this field should now have a caBundle corresponding to a TLS secret in the cluster. Create the webhook with:

```bash
kubectl apply -f deploy/validating-webhook-config.yaml
```

Once the reschedule hook container is running, `INFO Reschedule hook server started` should be logged during startup:

```bash
kubectl logs reschedule-hook-server
```

To test, try draining a node which hosts CAO managed pods. The drain command will continually attempt to evict the Couchbase pods while the operator handles moving the pods over to an uncordoned node. Once all Couchbase pods have been rescheduled, the node drain will be allowed to complete.

## Configuration

The reschedule hook can be configured using environment variables. Add these to the reschedule hook container template in your deployment using:

```yaml
env:
- name: POD_LABEL_SELECTOR_KEY
  value: "app"
```

### Available Configuration Options

| Environment Variable | Default Value | Description |
|---------------------|---------------|-------------|
| `POD_LABEL_SELECTOR_KEY` | `app` | Label selector key used to identify pods that should be handled by the reschedule hook
| `POD_LABEL_SELECTOR_VALUE` | `couchbase` | Value for the above key
| `RESCHEDULE_ANNOTATION_KEY` | `cao.couchbase.com/reschedule` | Key for the annotation added to pods for which requests are handled and have the above label, in order to mark them for rescheduling by an associated operator
| `RESCHEDULE_ANNOTATION_VALUE` | `true` | Value for the above key
| `TLS_CERT_FILE` | `/etc/webhook/certs/tls.crt` | Path to the mounted TLS certificate file
| `TLS_KEY_FILE` | `/etc/webhook/certs/tls.key` | Path to the mounted TLS private key file
| `TRACK_RESCHEULED_PODS` | `true` | Whether to track pods for which the reschedule annotation has already been added. Required in environments where pods might be recreated with the same name. If set to `false`, the `ClusterRole` will only need `get` and `patch` permissions for the `pods` resource
| `TRACKING_RESOURCE_TYPE` | `couchbasecluster` | Resource type used for tracking already rescheduled pods. Only effective if `TRACK_RESCHEULED_PODS` is `true`. Currently supports `couchbasecluster` and `namespace` resource types, for which the `ClusterRole` will require `get` and `patch` permissions

Note: To add support for additional tracking resource types, consider contributing to the project.

## Contributing

Couchbase welcomes anyone that wants to help out, whether that includes improving documentation or contributing code to fix bugs, increase test coverage, add additional features or anything in between. See the [contributing](CONTRIBUTE.md) document for more details.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) for details.

<div style="text-align: center; margin-top: 2em;">
<a href="https://www.couchbase.com/" style="text-decoration: none; color: inherit;"><img src="https://www.couchbase.com/wp-content/uploads/2023/10/couchbase-favicon.svg" alt="Couchbase Autonomous Operator" style="width: 4em; height: 4em; vertical-align:middle;"><span style="margin-left: 0.5em; font-weight: bold; font-size: 1.2em;">Couchbase</span></a>
</div>
