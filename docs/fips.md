# FIPS 140-3 in Red Hat Trusted Artifact Signer

## Overview

Red Hat Trusted Artifact Signer (RHTAS) is Red Hat Designed for FIPS. When deployed on a FIPS-enabled OpenShift cluster, RHTAS automatically activates FIPS mode and restricts all cryptographic operations to FIPS-validated algorithms. No manual FIPS configuration is required; however, some components (such as the database backend) may need to be configured to use FIPS-compatible options. See [Supported Databases](#supported-databases) for details.

On non-FIPS clusters, these restrictions do not apply and all cryptographic algorithms are available.

> **Note:** Some internal cluster communication between operator-managed components may not use TLS and may be transmitted unencrypted. External client traffic is protected via TLS at the Ingress or Route level.

## Supported Key Types and Algorithms

When FIPS mode is active, RHTAS accepts the following key types:

| Key Type | Minimum Size |
|----------|-------------|
| ECDSA (P-256, P-384, P-521) | -- |
| RSA | 2048 bits |
| Ed25519 | -- |

Client signing requests to Fulcio and Rekor are restricted to: ECDSA (P-256 with SHA-256, P-384 with SHA-384, P-521 with SHA-512), RSA PKCS#1 v1.5 (with SHA-256), Ed25519, and Ed25519ph.

For operator-level validation of user-provided keys and certificates, additional signature algorithms are also accepted: RSA PKCS#1 v1.5 (with SHA-384/512) and RSA-PSS (with SHA-256/384/512).

SHA-1 and MD5 are not permitted for signing or certificate validation in FIPS mode.

File-based signers must use PKCS#1, PKCS#8, or SEC1 (EC) PEM key formats. OpenSSH and Cosign key formats are not supported in FIPS mode.

## Supported Databases

Trillian (the transparency log backend) requires SHA-256 based authentication. Legacy authentication methods using SHA-1 or MD5 are not supported in FIPS mode.

| Database | Minimum Version | Auth Method | Supported |
|----------|----------------|-------------|-----------|
| PostgreSQL | 13+ | scram-sha-256 (must be configured; default in 14+) | Yes |
| MySQL | 8.0+ | caching_sha2_password (default) | Yes |
| MariaDB (all versions) | -- | mysql_native_password (SHA-1, protocol default) | No |

> **Warning:** The MariaDB instance shipped with RHTAS uses `mysql_native_password` (SHA-1) and is not suitable for FIPS mode. To run RHTAS in FIPS mode, configure an external MySQL 8.0+ or PostgreSQL 13+ database instead. See [Configuring an External Database](external-database.md) for setup instructions.

> **Note:** MariaDB is not compatible with FIPS mode. MariaDB's connection protocol requires SHA-1 authentication during the initial handshake, even when a SHA-256 authentication plugin is available. This is a protocol-level incompatibility that cannot be resolved through server configuration. In `fips140=only` strict mode, this causes a panic; in the default mode, the connection may succeed but use non-FIPS-validated cryptography.

> **Note:** MySQL connections must use TLS to meet FIPS requirements. Without TLS, MySQL's `caching_sha2_password` authentication uses a password exchange that is not FIPS-validated. To enable TLS, configure the `trustedCA` field on the Trillian specification with a ConfigMap containing the database server's CA certificate. See [Configuring an External Database](external-database.md) for details. On OpenShift, the operator automatically configures TLS for the managed database using service-serving certificates.

## Cryptographic Material Validation

When FIPS mode is active, the operator validates all user-provided keys, certificates, and TUF keys against the [supported algorithms](#supported-key-types-and-algorithms) before any component uses them.

### Password-Protected Keys

Password-protected private keys (legacy PEM-encrypted keys) are not supported in FIPS mode. The encryption scheme used by these keys relies on algorithms that are not FIPS-validated.

### KMS Providers

When a component is configured to use a KMS-based signer such as AWS or a Tink signer, the operator does not validate the signer key. These providers are responsible for their own cryptographic protections, and the key does not pass through the operator.

## FIPSCompliant Status Condition

When FIPS mode is active, each component custom resource includes a `FIPSCompliant` status condition that reports the result of cryptographic material validation:

```yaml
status:
  conditions:
    - type: FIPSCompliant
      status: "True"
      reason: FIPSValid
```

If validation fails, the condition shows the specific issue:

```yaml
status:
  conditions:
    - type: FIPSCompliant
      status: "False"
      reason: Failure
      message: "FIPS validation failed for spec.signer.keyRef: private key does not use a FIPS-approved algorithm: RSA key is 1024 bits, FIPS requires >= 2048 bits"
```

## FIPS Strict Mode (Testing Only)

By default, RHTAS runs with `fips140=auto`, which automatically enables FIPS mode when deployed on a FIPS-enabled cluster. Go also provides a stricter `fips140=only` mode, which causes non-FIPS cryptographic operations to return errors or panic instead of silently succeeding.

> **Warning:** `fips140=only` is **not intended for production use**. The Go documentation describes it as a "best effort mode meant for testing, assessment, and debugging" that "introduces crashes and potentially unhandled errors by design" and "may have false positives or false negatives."

To enable strict mode for testing, you can either:

**Option 1: Set the `GODEBUG` environment variable on the operator pod.**

For a quick test, use `oc set env`:

```sh
oc set env deployment/rhtas-operator-controller-manager GODEBUG=fips140=only -n openshift-rhtas-operator
```

If installed via OLM, add it to the Subscription:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhtas-operator
spec:
  config:
    env:
      - name: GODEBUG
        value: "fips140=only"
```

The operator propagates its `GODEBUG` value to all managed workload containers.

**Option 2: Set the `rhtas.redhat.com/godebug` annotation on the Securesign custom resource.**

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Securesign
metadata:
  annotations:
    rhtas.redhat.com/godebug: "fips140=only"
```

This annotation is inherited by all child components (Rekor, Fulcio, CTlog, Trillian, TSA, TUF) and takes precedence over the operator's environment variable.

To revert to the default behavior, remove the override.

## Component Behavior in FIPS Mode

### Fulcio

- Client signing requests are restricted to FIPS-approved algorithms only. Requests using non-approved algorithms are rejected.

### Rekor

The following features are disabled in FIPS mode:

| Feature | Reason |
|---------|--------|
| Helm, RPM entry types | Depend on PGP, which is not FIPS-approved |
| Alpine entry type | Uses SHA-1 for signature verification |
| PGP, SSH, Minisign signature formats | Use unvalidated crypto modules |

Client signing requests are restricted to FIPS-approved algorithms only.

The Rekor CLI defaults to `x509` format instead of `pgp` when FIPS mode is active.

### Cosign

- Password-protected cosign keys cannot be created or loaded. In FIPS mode, cosign generates unencrypted keys instead.
- Storing secrets to GitHub Actions is not available.
- Azure KMS with certificate-based authentication is not available.

### Gitsign

- Only SHA-2 and SHA-3 hash algorithms are accepted for commit signatures and timestamps.

### Timestamp Authority (TSA)

- Sigstore-encrypted and cosign-encrypted private key formats are not supported.
