# RHTAS Operator

Red Hat Trusted Artifact Signer (RHTAS) operator deploys a production-ready [Sigstore](https://www.sigstore.dev/) infrastructure on OpenShift and Kubernetes.

## Description

Red Hat Trusted Artifact Signer enhances software supply chain security by simplifying cryptographic signing and verification of software artifacts, such as container images, binaries, and documents. It provides a production-ready deployment of the Sigstore project within an enterprise. Enterprises adopting it can meet signing-related criteria for achieving Supply Chain Levels for Software Artifacts (SLSA) compliance and have greater confidence in the security and trustworthiness of their software supply chains.

## Installation

- [OpenShift](docs/openshift.md) — Install from OperatorHub or via kustomize
- [Kubernetes](docs/kubernetes.md) — Install via kustomize (EKS included)

> **Vanilla Kubernetes at a glance:** the operator is OpenShift-first but runs on
> generic Kubernetes (kind, EKS, GKE, ...). Note the prerequisites and differences,
> all covered in the [Kubernetes guide](docs/kubernetes.md):
> - **Server-side apply is required** (`kubectl apply --server-side -k ...`) — the
>   `securesigns` CRD exceeds the 256 KB client-side annotation limit.
> - **cert-manager is required** (operator webhooks); an **Ingress controller** is
>   required only for external access; the **Prometheus Operator** is optional
>   (`ServiceMonitor` creation is skipped gracefully when absent).
> - **Public-registry images** are provided by the `kubernetes` overlay so no
>   `registry.redhat.io` credentials are needed; overridable per image.
> - The OpenShift **`ConsoleCLIDownload`** integration is OpenShift-only; set
>   `externalAccess.host` (or `--ingress-host-template`) for routable hostnames.
>
> OpenShift behaviour is unchanged and auto-detected at startup.

## Getting Started

Once the operator is installed, deploy the signing infrastructure by creating a Securesign CR. You will need an OIDC provider (e.g., Keycloak, Amazon Cognito).

1. Modify the sample CR for your environment (OIDC issuer, certificate details, external access):

```sh
kubectl apply -f config/samples/rhtas_v1alpha1_securesign.yaml -n <operator-namespace>
```

2. The operator deploys Fulcio, Rekor, Trillian, CTlog, TUF, and optionally a Timestamp Authority.

3. Initialize the TUF root of trust:

```sh
cosign initialize --mirror=https://tuf.<your-domain>/ --root=https://tuf.<your-domain>/root.json
```

4. Sign an image (Fulcio and Rekor URLs are resolved from TUF configuration):

```sh
cosign sign -y <image>
```

5. Verify signatures:

```sh
cosign verify --certificate-identity-regexp ".*@example" \
  --certificate-oidc-issuer-regexp ".*keycloak.*" <image>
```

## Components

| Component | Description |
|---|---|
| **Fulcio** | Issues code-signing certificates based on OIDC identity |
| **Rekor** | Transparency log recording signatures and attestations |
| **Trillian** | Backend Merkle tree for Rekor and CTlog |
| **CTlog** | Certificate transparency log for Fulcio certificates |
| **TUF** | Distributes and rotates cryptographic trust roots |
| **TSA** | RFC 3161 timestamp authority (optional) |

## Uninstall

See the uninstall section in the [OpenShift](docs/openshift.md#uninstall) or [Kubernetes](docs/kubernetes.md#uninstall) guide.

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md) for building, testing, and contributing.
