# Installing on Kubernetes

The operator is OpenShift-first but runs on generic/vanilla Kubernetes (kind, EKS,
GKE, ...). This page documents the vanilla-Kubernetes prerequisites, limitations,
and the differences from an OpenShift install. OpenShift behaviour is unchanged and
is auto-detected at startup (see [openshift.md](openshift.md)).

## Prerequisites

| Requirement | Needed for | Notes |
| --- | --- | --- |
| **cert-manager** | Operator webhooks (required) | The `kubernetes` overlay pulls in cert-manager `Certificate`/`Issuer` resources to serve the validating/mutating webhooks. Install cert-manager **before** applying the overlay. |
| **Ingress controller** | External access (optional) | Required only if you set `externalAccess.enabled: true` on a component. Any controller works (ingress-nginx, etc.). Set a default `IngressClass` or a `host`. |
| **Prometheus Operator** | Metrics scraping (optional) | Provides the `monitoring.coreos.com` `ServiceMonitor` API. If absent and `monitoring.enabled: true`, reconciliation fails with a clear error (see [Monitoring](#monitoring)). |
| **Image pull access** | Pulling component images | Vanilla clusters usually lack `registry.redhat.io` credentials. Use the public-registry overlay images (see [Image overrides](#image-overrides)). |

## Server-side apply is required

The `securesigns` CRD is larger than the 256 KB limit of the client-side
`kubectl.kubernetes.io/last-applied-configuration` annotation, so a normal
(client-side) `kubectl apply` fails with `metadata.annotations: Too long`. Always
use **server-side apply** (`--server-side`). The Makefile `install`/`deploy`
targets already do this.

## Install via kustomize

Install cert-manager first, then apply the overlay with server-side apply:

```sh
kubectl apply --server-side --kustomize https://github.com/securesign/secure-sign-operator/config/overlays/kubernetes?ref=<tag-or-branch>
```

Or clone the repository and apply locally:

```sh
git clone --branch <tag> https://github.com/securesign/secure-sign-operator.git
cd secure-sign-operator
kubectl apply --server-side --kustomize config/overlays/kubernetes
```

## Verify

```sh
kubectl get pods -n rhtas-operator
```

The operator pod should be `1/1 Running`.

## Image overrides

All component images default to `registry.redhat.io` (the OpenShift/product
registry), which requires credentials. On vanilla Kubernetes you must override the
images that have no unauthenticated pull path. Set the matching `RELATED_IMAGE_*`
environment variable on the operator Deployment. Precedence is flag > env var >
built-in default.

Most images have public digest mirrors under `quay.io/securesign` (identical
digests to the product images). The `httpd` base image is available from the
unauthenticated `registry.access.redhat.com`.

```sh
kubectl -n rhtas-operator set env deployment/rhtas-operator-controller-manager \
  RELATED_IMAGE_TRILLIAN_LOG_SIGNER=quay.io/securesign/trillian-logsigner@sha256:... \
  RELATED_IMAGE_TRILLIAN_DB=quay.io/securesign/trillian-database@sha256:... \
  # ... one variable per component image
```

The exact digest values for the current release are in
[`config/default/images.env`](../config/default/images.env). Substitute
`registry.redhat.io/rhtas/<name>-rhel9` → `quay.io/securesign/<name>` for each
image (digests are identical).

The product default registry in `config/default/images.env` is intentionally left
unchanged, so OpenShift/product builds keep using `registry.redhat.io`.

> **Note — Trillian DB readiness (netcat):** `RELATED_IMAGE_TRILLIAN_NETCAT`
> (`registry.redhat.io/openshift4/ose-tools-rhel9`) has no free public mirror. It is
> used only as the Trillian DB readiness init container, which runs
> `nc -z -v -w30` (so a busybox `nc`, which lacks `-v`, is not a drop-in
> replacement). On a cluster without `registry.redhat.io` credentials, either supply
> a pull secret for this one image or override `RELATED_IMAGE_TRILLIAN_NETCAT` with a
> public `ncat`-capable image. This affects only Trillian with a self-created
> database (`trillian.database.create: true`).

## External access and hostnames

`Route` objects are OpenShift-only. On vanilla Kubernetes, components with
`externalAccess.enabled: true` are exposed via an `Ingress` (the OpenShift-only
`route.openshift.io/termination: edge` annotation and auto-TLS are not added off
OpenShift). You therefore need an Ingress controller, and you should set a hostname:

- Set `externalAccess.host` on each component to the DNS name you will route to, **or**
- Set the operator-wide hostname template with the `--ingress-host-template` flag /
  `INGRESS_HOST_TEMPLATE` env var. The default template is `%[1]s.local`
  (`%[1]s` = service name, `%[2]s` = namespace), which is suitable for
  port-forwarding / local testing but is not a routable DNS name. For real DNS use a
  template such as `%[1]s.%[2]s.<your-domain>` or a wildcard service like
  `%[1]s.%[2]s.<ingress-ip>.nip.io`.

## Monitoring

`ServiceMonitor` (`monitoring.coreos.com/v1`) is a Prometheus Operator API. The
operator auto-detects it at startup:

- If the Prometheus Operator is **installed**, components with `monitoring.enabled:
  true` get a `ServiceMonitor`.
- If it is **absent** but `monitoring.enabled: true`, reconciliation **fails** with a
  clear error condition directing you to install the Prometheus Operator or set
  `monitoring.enabled: false`. This ensures the CR spec is never silently ignored.

You can force the detection result with the `--monitoring` flag / `MONITORING` env var.
Set `monitoring.enabled: false` on each component to opt out of `ServiceMonitor`
creation.

The operator's own metrics are exposed on port 8443 over HTTPS with bearer token
authentication. The TLS certificate is self-signed and auto-generated by the operator.

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
kubectl delete --kustomize config/overlays/kubernetes
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
Kubernetes there is no console download link. The CLI binaries are instead served by
the `cli-server` Service (namespace `trusted-artifact-signer`), which is reachable on
both platforms.

For quick local access without an Ingress, port-forward the Service:

```sh
kubectl -n trusted-artifact-signer port-forward svc/cli-server 8080:8080
# then e.g. download cosign for linux/amd64:
curl -sSL http://localhost:8080/clients/linux/cosign-amd64.gz | gunzip > cosign
```

To access cosign, gitsign, rekor-cli, and ec binaries from outside the cluster, create an Ingress:

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
