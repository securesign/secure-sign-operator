# Installing on Kubernetes

The operator is OpenShift-first but runs on generic/vanilla Kubernetes (kind, EKS,
GKE, ...). This page documents the vanilla-Kubernetes prerequisites, limitations,
and the differences from an OpenShift install. OpenShift behaviour is unchanged and
is auto-detected at startup (see [openshift.md](openshift.md)).

## Prerequisites

### cert-manager

The operator webhooks require [cert-manager](https://cert-manager.io) for TLS certificate management.
Install it before applying the operator overlay:

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

### Ingress controller

Required only if you set `ingress.enabled: true` on a component. Any
controller works (ingress-nginx, Traefik, etc.). See
[External access and hostnames](#external-access-and-hostnames) for hostname
configuration.

### Prometheus Operator (optional)

The `config/overlays/kubernetes` overlay does **not** include a `ServiceMonitor`
resource. The operator's own metrics `ServiceMonitor` is included only in the
OpenShift overlay (`config/overlays/openshift`), so the kubernetes overlay deploys
cleanly without Prometheus Operator installed.

If `monitoring.enabled: true` (the default) and the Prometheus Operator is **not**
installed, reconciliation **fails** with a clear error directing you to install
the Prometheus Operator or set `monitoring.enabled: false`. See
[Monitoring](#monitoring) for details.

## Image Registry

All operator and component images are pulled from `registry.redhat.io`, which requires
authentication. Unauthenticated pulls will fail with `ImagePullBackOff`.

### Create a pull secret

Create a pull secret in the namespace where TAS components will run using your Red Hat
registry service account credentials. Create a non-expiring service account token at
[console.redhat.com/iam/service-accounts](https://console.redhat.com/iam/service-accounts)
(see [Red Hat Container Registry Authentication](https://access.redhat.com/articles/RegistryAuthentication)
and [Registry Service Account management](https://access.redhat.com/terms-based-registry/)):

```sh
kubectl create secret docker-registry redhat-registry \
  --docker-server=registry.redhat.io \
  --docker-username=<your-username-or-sa-name> \
  --docker-password=<your-password-or-token> \
  -n <tas-namespace>
```

### Reference the pull secret in each TAS component

Add `imagePullSecrets` to every component in your `Securesign` CR spec. The field is
available on all six components:

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Securesign
metadata:
  name: securesign
  namespace: <tas-namespace>
spec:
  fulcio:
    imagePullSecrets:
      - name: redhat-registry
    # ... rest of fulcio config
  rekor:
    imagePullSecrets:
      - name: redhat-registry
    # ...
  ctlog:
    imagePullSecrets:
      - name: redhat-registry
    # ...
  trillian:
    imagePullSecrets:
      - name: redhat-registry
    # ...
  tuf:
    imagePullSecrets:
      - name: redhat-registry
    # ...
  tsa:
    imagePullSecrets:
      - name: redhat-registry
    # ...
```

The operator propagates `imagePullSecrets` to every `Pod` it creates for that component,
including database and signer pods.

> **Alternatives:**
> - For local development, `make dev-images` (documented in `DEVELOPMENT.md`)
>   builds images from source and avoids the registry entirely.
> - For clusters where per-namespace secrets are impractical, add the pull secret
>   to the default `ServiceAccount` in each namespace, or configure it at the
>   node level (see [Red Hat Container Registry Authentication](https://access.redhat.com/articles/RegistryAuthentication)).
> - For air-gapped or mirror-based setups, configure cluster-level registry
>   mirroring to redirect `registry.redhat.io` pulls to an internal mirror.

## External access and hostnames

`Route` objects are OpenShift-only. On vanilla Kubernetes, components with
`ingress.enabled: true` are exposed via an `Ingress`. You therefore need
an Ingress controller and must configure hostnames.

Two ways to set hostnames:

- **Per component** — set `ingress.host` on each component in the
  `Securesign` CR.
- **Operator-wide template** — set the `--ingress-host-template` flag or the
  `INGRESS_HOST_TEMPLATE` env var on the operator Deployment. The template uses
  `fmt.Sprintf`-style positional arguments:
  - `%[1]s` = service name
  - `%[2]s` = namespace

  The default is `%[1]s.local`, suitable for port-forwarding / local testing but
  not a routable DNS name. For real DNS use a pattern such as
  `%[1]s.%[2]s.<your-domain>` or a wildcard-DNS service like
  `%[1]s.%[2]s.<ingress-ip>.nip.io`.

## Install via kustomize

Server-side apply is required because the `securesigns` CRD exceeds the 256 KB
client-side annotation limit.

Apply directly from the repository:

```sh
kubectl apply --server-side --kustomize \
  https://github.com/securesign/secure-sign-operator/config/overlays/kubernetes?ref=<tag-or-branch>
```

Or clone the repository and apply locally:

```sh
git clone --branch <tag> https://github.com/securesign/secure-sign-operator.git
cd secure-sign-operator
kubectl apply --server-side -k config/overlays/kubernetes
```

## Verify

```sh
kubectl get pods -n openshift-rhtas-operator
```

The operator pod should be `1/1 Running`.

## Monitoring

`ServiceMonitor` (`monitoring.coreos.com/v1`) is a Prometheus Operator API. If
`monitoring.enabled: true` (the default) but the Prometheus Operator is **not**
installed, the monitoring `Handle()` action returns a clear error:

```
monitoring.enabled is true but ServiceMonitor CRD is not installed;
install the Prometheus Operator or set monitoring.enabled=false
```

The error self-heals — once the Prometheus Operator is installed the next
reconciliation succeeds without restarting the operator (the RESTMapper cache
auto-invalidates when CRDs are added).

To opt out of `ServiceMonitor` creation entirely, set `monitoring.enabled: false`
on each component in the `Securesign` CR:

```yaml
spec:
  fulcio:
    monitoring:
      enabled: false
  rekor:
    monitoring:
      enabled: false
  # ... repeat for ctlog, trillian, tuf, tsa
```

## Metrics

The operator exposes its own metrics on port 8443 over HTTPS with bearer token
authentication. The TLS certificate is self-signed and auto-generated.

To scrape metrics with Prometheus, configure your ServiceMonitor or scrape config to:
- Use `scheme: https` with `insecureSkipVerify: true`
- Include a bearer token from a ServiceAccount bound to the `rhtas-operator-metrics-reader` ClusterRole

```sh
kubectl create clusterrolebinding prometheus-metrics-reader \
  --clusterrole=rhtas-operator-metrics-reader \
  --serviceaccount=<prometheus-namespace>:<prometheus-sa>
```

## Uninstall

```sh
kubectl delete -k config/overlays/kubernetes
```

## EKS

RHTAS runs on Amazon EKS. If image building and signing all occurs within the cluster, Ingress and Certificates are not required. To verify signatures from outside the cluster, deploy with Ingress and Certificates.

A script at `ci/eks.sh` in the source repository can provision a suitable EKS cluster.

After the operator is running, create a namespace and apply a Securesign CR configured for your environment:

```sh
kubectl create ns securesign
kubectl apply --server-side -n securesign -f config/samples/rhtas_v1alpha1_securesign.yaml
```

See `config/samples/` for example CR configurations with OIDC providers and external access.

### Client Binaries

The OpenShift `ConsoleCLIDownload` integration is **OpenShift-only**; on vanilla
Kubernetes there is no console download link. The CLI binaries are served by the
`cli-server` Service and are reachable on both platforms.

For quick local access without an Ingress, port-forward the Service:

```sh
kubectl -n securesign port-forward svc/cli-server 8080:8080
# then e.g. download cosign for linux/amd64:
curl -sSL http://localhost:8080/clients/linux/cosign-amd64.gz | gunzip > cosign
```

To expose the CLI server externally, create an Ingress:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: cli-external
  namespace: securesign
spec:
  ingressClassName: nginx
  rules:
  - host: cli-server.example.com
    http:
      paths:
      - backend:
          service:
            name: cli-server
            port:
              name: cli-server
        path: /clients(/|$)(.*)
        pathType: ImplementationSpecific
```
