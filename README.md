# Couchbase Reschedule Hook


A Kubernetes webhook that manages the rescheduling of Couchbase pods during cluster operations like node draining and upgrades.

## Overview

The Couchbase Reschedule Hook is a Kubernetes admission webhook that intercepts pod eviction requests and ensures graceful handling of Couchbase pods during node drain events. It works in conjunction with the Couchbase Autonomous Operator to manage pod rescheduling in a controlled manner by adding the `cao.couchbase.com/reschedule` annotation to pods being evicted. The Couchbase Autonomous Operator will gracefull move pods with this annotation to uncordoned nodes. If pods will be rescheduled onto other nodes with the same pod name, this webhook will use annotations on the pods' parent CouchbaseCluster resource to track pod rescheduling progress.

## Installation

1. Generate TLS certificates for the webhook:
```bash
./scripts/generate-certs.sh
```

2. Deploy the webhook:
```bash
kubectl apply -f deploy/reschedule-hook-deployment.yaml
```

## How It Works

1. When a pod eviction request is received, the webhook checks if the pod is a Couchbase pod
2. If it is a Couchbase pod:
   - For pods not yet marked for rescheduling, adds a reschedule annotation
   - For pods already marked for rescheduling, denies the eviction until rescheduling is complete
   - If the CouchbaseCluster a pod belongs to has an upgradeProcess of InPlaceUpgrade, a tracking annotation will be added to the CouchbaseCluster to handle pods which will be rescheduled with the same name.
3. For non-Couchbase pods, allow the eviction to continue normally

## Development

### Building

```bash
make build
```

### Running Tests

```bash
make lint
```

### Building Docker Image

```bash
make docker-build
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
