# Development Guide

## Building

```sh
# Build the operator binary
make build

# Build and push container image
make docker-build docker-push IMG=<registry>/operator:tag

# Cross-platform build (e.g., linux/amd64 from an ARM machine)
PLATFORMS=linux/amd64 make docker-buildx IMG=<registry>/operator:tag
```

## Running and Deploying

There are three ways to run the operator for development and testing:

### Option 1: Run locally (fastest iteration)

The operator runs as a local process on your machine, connecting to the cluster via your kubeconfig. No image build required — ideal for rapid development.

```sh
make install   # install CRDs
make run       # run operator locally
```

The operator watches the cluster and reconciles resources. Stop with `Ctrl+C`. CRDs remain installed.

### Option 2: Deploy to cluster via kustomize

The operator runs as a pod inside the cluster. Requires building and pushing an image first.

```sh
make docker-build docker-push IMG=<registry>/operator:tag
make deploy IMG=<registry>/operator:tag
```

Use `TARGET_PLATFORM` to select the platform overlay (see [Platform Selection](#platform-selection)).

### Option 3: Deploy via OLM bundle

Tests the full OLM install path including the CSV, RBAC, and service configuration as end users would experience it.

```sh
make bundle IMG=<registry>/operator:tag
make bundle-build bundle-push BUNDLE_IMG=<registry>/operator-bundle:tag
operator-sdk run bundle <registry>/operator-bundle:tag --namespace openshift-rhtas-operator --timeout 5m
```

## Cleanup

| Method | Uninstall command |
|---|---|
| Local run (`make run`) | `Ctrl+C` to stop, then `make uninstall` to remove CRDs |
| Kustomize (`make deploy`) | `make undeploy` to remove all resources, `make uninstall` to remove CRDs |
| OLM bundle | `operator-sdk cleanup rhtas-operator --namespace openshift-rhtas-operator` |

## Code Generation

After modifying API types in `api/v1alpha1/*_types.go`:

```sh
make manifests generate
```

This regenerates CRDs, RBAC manifests, and deepcopy methods.

## Testing

```sh
# Unit tests
make test

# E2E tests (sequential, requires a running cluster)
go test -p 1 ./test/e2e/... -tags=integration -timeout 30m
go test -p 1 ./test/e2e/... -tags=upgrade -timeout 20m
go test -p 1 ./test/e2e/... -tags=ha -timeout 20m
go test -p 1 ./test/e2e/... -tags=custom_install -timeout 20m
```

## Linting

```sh
make lint
make lint-fix
```

## Platform Selection

The `TARGET_PLATFORM` variable controls which kustomize overlay is used for deployment and bundle generation:

| Value | Description |
|---|---|
| `openshift` (default) | OpenShift overlay with service-serving-cert TLS, Prometheus RBAC, and ServiceMonitor with CA verification |
| `kubernetes` | Plain Kubernetes overlay with self-signed TLS and basic ServiceMonitor |

Usage:

```sh
# Deploy to OpenShift (default)
make deploy IMG=<registry>/operator:tag

# Deploy to Kubernetes
make deploy IMG=<registry>/operator:tag TARGET_PLATFORM=kubernetes

# Generate OpenShift bundle
make bundle IMG=<registry>/operator:tag

# Generate Kubernetes bundle
make bundle IMG=<registry>/operator:tag TARGET_PLATFORM=kubernetes
```

## Bundle Images

Build and push OLM bundle images:

```sh
make bundle IMG=<registry>/operator:tag
make bundle-build bundle-push BUNDLE_IMG=<registry>/operator-bundle:tag
```

For Kubernetes bundles, add `TARGET_PLATFORM=kubernetes` to all three commands.
