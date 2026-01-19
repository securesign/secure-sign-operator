# Configuring Rekor Attestation Storage

This guide describes how to configure attestation storage for Rekor in Red Hat Trusted Artifact Signer (RHTAS). Attestation storage allows Rekor to store detailed attestation data, which is essential for supply chain security and SLSA compliance. For production deployments, using cloud object storage is recommended over local file system storage.

## Overview

Attestation storage is an optional feature that allows Rekor to persist full attestation data separately from the transparency log entries. This enables retrieval of original attestation content when querying the log.

> **Important**: Not all Rekor entry types use attestation storage. Only specific entry types that contain attestation payloads are stored.

## Supported Entry Types

Only `intoto` and `cose` entry types store data in attestation storage. Attestations created with `cosign attest` use the `intoto` type and benefit from this feature. Signatures created with `cosign sign` use `hashedrekord` entries which are not stored.

Attestations have a maximum size limit (default: 100KB, configurable via `maxSize`). Attestations exceeding this limit are skipped.

## Storage Backends

Rekor supports storing attestations in various storage backends:

* **Local file system** - Uses a Persistent Volume Claim (PVC) mounted to the Rekor pod
* **Amazon S3** - Cloud object storage from AWS
* **S3-compatible storage** - MinIO or other S3-compatible services
* **Google Cloud Storage (GCS)** - Cloud object storage from Google Cloud
* **Azure Blob Storage** - Cloud object storage from Microsoft Azure

For production high availability deployments, using cloud object storage is recommended over local file system storage.

## Attestation Configuration Fields

The `attestations` section in the Rekor specification includes the following fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `true` | Enables or disables attestation storage. Once enabled, cannot be disabled. |
| `url` | string | `file:///var/run/attestations?no_tmp_dir=true` | Storage location URL using [go-cloud blob URL format](https://gocloud.dev/howto/blob/) |
| `maxSize` | quantity | `100Ki` | Maximum allowed size for an individual attestation |

> **Note**: Once attestation storage is enabled, it cannot be disabled. Disabling would break references from existing transparency log entries to their stored attestations, making it impossible to retrieve the full attestation content for those entries.

> **Note**: The `maxSize` value must not exceed the Rekor server's `maxRequestBodySize` configuration. The request body size limit controls the maximum size of incoming HTTP requests, so attestations larger than this limit will be rejected before they can be stored.

## Storage URL Formats

The `url` field supports the following protocols:

| Protocol | Example | Description |
|----------|---------|-------------|
| `file://` | `file:///var/run/attestations?no_tmp_dir=true` | Local filesystem (requires PVC) |
| `s3://` | `s3://bucket-name?region=us-west-1` | Amazon S3 |
| `s3://` | `s3://bucket?endpoint=minio.local:9000&use_path_style=true` | S3-compatible (MinIO) |
| `gs://` | `gs://bucket-name` | Google Cloud Storage |
| `azblob://` | `azblob://container-name` | Azure Blob Storage |
| `mem://` | `mem://` | In-memory (development/testing only) |

## Using Local File System Storage

Local file system storage uses a PVC mounted to the Rekor pod. This is the default configuration but has limitations for high availability deployments.

### Basic Configuration

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  attestations:
    enabled: true
    url: "file:///var/run/attestations?no_tmp_dir=true"
    maxSize: "100Ki"
  pvc:
    size: "5Gi"
    retain: true
    accessModes:
      - ReadWriteOnce
```

### High Availability with File Storage

> **Important**: When using file-based attestation storage with multiple Rekor replicas, the PVC must support ReadWriteMany (RWX) access mode. The operator validates this constraint and rejects configurations that violate it.

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "file:///var/run/attestations?no_tmp_dir=true"
    maxSize: "100Ki"
  pvc:
    size: "10Gi"
    retain: true
    accessModes:
      - ReadWriteMany  # Required for replicas > 1
    storageClass: "ocs-storagecluster-cephfs"  # Must support RWX
```

### Limitations of File-based Storage

* Requires ReadWriteMany (RWX) storage class for HA deployments
* Storage capacity limited by PVC size
* Performance may be impacted by storage backend
* Less suitable for multi-region deployments

## Using Amazon S3

Amazon S3 is the recommended storage backend for production deployments due to its scalability, durability, and availability.

### Prerequisites

1. An S3 bucket created in your AWS account
2. IAM credentials with permissions to read/write to the bucket
3. Network connectivity from the OpenShift cluster to AWS S3

### Creating S3 Credentials Secret

Create a secret containing your AWS credentials:

```bash
oc create secret generic rekor-s3-credentials \
  --from-literal=AWS_ACCESS_KEY_ID="your-access-key" \
  --from-literal=AWS_SECRET_ACCESS_KEY="your-secret-key" \
  -n trusted-artifact-signer
```

### Configuring Rekor with S3

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "s3://my-attestation-bucket?region=us-east-1"
    maxSize: "100Ki"
  auth:
    env:
      - name: AWS_ACCESS_KEY_ID
        valueFrom:
          secretKeyRef:
            name: rekor-s3-credentials
            key: AWS_ACCESS_KEY_ID
      - name: AWS_SECRET_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: rekor-s3-credentials
            key: AWS_SECRET_ACCESS_KEY
```

### S3 URL Parameters

The S3 URL supports various query parameters:

| Parameter | Description | Example |
|-----------|-------------|---------|
| `region` | AWS region | `region=us-east-1` |
| `endpoint` | Custom endpoint for S3-compatible storage | `endpoint=s3.example.com` |
| `use_path_style` | Use path-style URLs (required for MinIO) | `use_path_style=true` |
| `disableSSL` | Disable SSL (not recommended for production) | `disableSSL=true` |

### Example: S3 with Custom Endpoint (MinIO)

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "s3://attestations?endpoint=minio.example.com:9000&use_path_style=true"
    maxSize: "100Ki"
  auth:
    env:
      - name: AWS_ACCESS_KEY_ID
        valueFrom:
          secretKeyRef:
            name: minio-credentials
            key: access-key
      - name: AWS_SECRET_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: minio-credentials
            key: secret-key
```

## Using Google Cloud Storage

Google Cloud Storage provides a highly durable and available storage option for GCP environments.

### Prerequisites

1. A GCS bucket created in your GCP project
2. Service account with Storage Object Admin permissions
3. Service account key file

### Creating GCS Credentials Secret

```bash
oc create secret generic rekor-gcs-credentials \
  --from-file=service-account.json=/path/to/service-account-key.json \
  -n trusted-artifact-signer
```

### Configuring Rekor with GCS

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "gs://my-attestation-bucket"
    maxSize: "100Ki"
  auth:
    env:
      - name: GOOGLE_APPLICATION_CREDENTIALS
        value: "/var/run/secrets/tas/auth/service-account.json"
    secretMount:
      - name: rekor-gcs-credentials
        key: service-account.json
```

> **Note**: The `secretMount` configuration mounts secrets at `/var/run/secrets/tas/auth` by default. The `GOOGLE_APPLICATION_CREDENTIALS` environment variable tells the Go Cloud SDK where to find the credentials file.

## Using Azure Blob Storage

Azure Blob Storage is the recommended option for Azure environments.

### Prerequisites

1. An Azure Storage Account
2. A container created in the storage account
3. Storage account access key or SAS token

### Creating Azure Credentials Secret

```bash
oc create secret generic rekor-azure-credentials \
  --from-literal=AZURE_STORAGE_ACCOUNT="your-storage-account" \
  --from-literal=AZURE_STORAGE_KEY="your-storage-key" \
  -n trusted-artifact-signer
```

### Configuring Rekor with Azure Blob Storage

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "azblob://my-attestation-container"
    maxSize: "100Ki"
  auth:
    env:
      - name: AZURE_STORAGE_ACCOUNT
        valueFrom:
          secretKeyRef:
            name: rekor-azure-credentials
            key: AZURE_STORAGE_ACCOUNT
      - name: AZURE_STORAGE_KEY
        valueFrom:
          secretKeyRef:
            name: rekor-azure-credentials
            key: AZURE_STORAGE_KEY
```

## Red Hat OpenShift Data Foundation Integration

Red Hat OpenShift Data Foundation (ODF) provides S3-compatible object storage through NooBaa, which can be used for attestation storage.

### Prerequisites

1. OpenShift Data Foundation installed and configured
2. Object Bucket Claim created

### Creating Object Bucket Claim

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: rekor-attestations-bucket
  namespace: trusted-artifact-signer
spec:
  generateBucketName: rekor-attestations
  storageClassName: openshift-storage.noobaa.io
```

### Using ODF Object Storage

After the ObjectBucketClaim is created, retrieve the bucket name and credentials:

```bash
# Get bucket name
BUCKET_NAME=$(oc get obc rekor-attestations-bucket -o jsonpath='{.spec.bucketName}')

# Get endpoint
S3_ENDPOINT=$(oc get route s3 -n openshift-storage -o jsonpath='{.spec.host}')
```

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  replicas: 3
  attestations:
    enabled: true
    url: "s3://rekor-attestations-xyz?endpoint=s3-openshift-storage.apps.cluster.example.com&use_path_style=true"
    maxSize: "100Ki"
  auth:
    env:
      - name: AWS_ACCESS_KEY_ID
        valueFrom:
          secretKeyRef:
            name: rekor-attestations-bucket  # Auto-generated secret
            key: AWS_ACCESS_KEY_ID
      - name: AWS_SECRET_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: rekor-attestations-bucket  # Auto-generated secret
            key: AWS_SECRET_ACCESS_KEY
```

## Limitations

- Only `intoto` and `cose` entry types store attestations.
- Once enabled, attestation storage cannot be disabled.
- When using `file://` protocol, the URL must start with `file:///var/run/attestations`.
- Cloud storage backends require network connectivity from the cluster.

## Related Documentation

* [High Availability Overview](./high-availability-overview.md)
* [Configuring External Search Index](./external-search-index.md)
* [Configuring RWX Storage](./pvc-rwx-storage.md)
