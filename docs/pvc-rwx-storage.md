# Configuring ReadWriteMany (RWX) Storage for High Availability

This guide describes the ReadWriteMany (RWX) storage requirements for high availability deployments of Red Hat Trusted Artifact Signer (RHTAS). Understanding these requirements is essential for successful HA deployment.

## Overview

For high availability deployments with multiple replicas distributed across different nodes, some RHTAS components require **ReadWriteMany (RWX)** access mode for their persistent storage. RWX allows the volume to be mounted as read-write by multiple nodes simultaneously.

For more information about PVC access modes, see the [OpenShift Storage documentation](https://docs.openshift.com/container-platform/latest/storage/understanding-persistent-storage.html#pv-access-modes_understanding-persistent-storage).

## Components Requiring RWX Storage

### TUF Repository

The TUF (The Update Framework) component stores cryptographic metadata that must be accessible to all replicas. When running more than 1 TUF replica, RWX storage is required.

> **Note**: The operator validates that `accessModes` includes `ReadWriteMany` when `replicas > 1`. Configurations that do not meet this requirement will be rejected.

```yaml
spec:
  tuf:
    replicas: 3
    pvc:
      accessModes:
        - ReadWriteMany
      storageClass: "ocs-storagecluster-cephfs"
```

### Rekor File-based Attestation Storage

When using file-based attestation storage (`file://` URL) with multiple Rekor replicas, RWX storage is required for the attestation directory.

> **Note**: The operator validates that when attestations are enabled with `file://` URL and `replicas > 1`, the `accessModes` must include `ReadWriteMany`.

```yaml
spec:
  rekor:
    replicas: 3
    attestations:
      enabled: true
      url: "file:///var/run/attestations?no_tmp_dir=true"
    pvc:
      accessModes:
        - ReadWriteMany
      storageClass: "ocs-storagecluster-cephfs"
```

> **Recommendation**: For HA deployments, consider using cloud object storage (S3, GCS, Azure) for attestations instead of file-based storage. This eliminates the RWX requirement for Rekor.

## RWX Storage Options

Several storage solutions support RWX access mode. Configure the appropriate `storageClass` in your RHTAS deployment based on your environment:

| Environment | Storage Solution | Example Storage Class | Documentation |
|-------------|-----------------|----------------------|---------------|
| OpenShift | OpenShift Data Foundation (CephFS) | `ocs-storagecluster-cephfs` | [ODF Documentation](https://access.redhat.com/documentation/en-us/red_hat_openshift_data_foundation/) |
| OpenShift / Kubernetes | NFS | `nfs-client` | [NFS Provisioner](https://github.com/kubernetes-sigs/nfs-subdir-external-provisioner) |
| AWS | Amazon EFS | `efs-sc` | [EFS CSI Driver](https://docs.aws.amazon.com/eks/latest/userguide/efs-csi.html) |
| Azure | Azure Files | `azurefile` | [Azure Files CSI Driver](https://learn.microsoft.com/en-us/azure/aks/azure-files-csi) |
| GCP | Google Cloud Filestore | `filestore-rwx` | [Filestore CSI Driver](https://cloud.google.com/filestore/docs/csi-driver) |

### Configuration Example

Once you have an RWX-capable storage class configured in your cluster, reference it in the RHTAS configuration:

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Securesign
metadata:
  name: securesign-ha
  namespace: trusted-artifact-signer
spec:
  tuf:
    replicas: 3
    pvc:
      size: "100Mi"
      retain: true
      accessModes:
        - ReadWriteMany
      storageClass: "<your-rwx-storage-class>"

  rekor:
    replicas: 3
    attestations:
      enabled: true
      url: "file:///var/run/attestations?no_tmp_dir=true"
    pvc:
      size: "5Gi"
      retain: true
      accessModes:
        - ReadWriteMany
      storageClass: "<your-rwx-storage-class>"
```

For more information about storage options on OpenShift, see the [OpenShift Storage documentation](https://docs.openshift.com/container-platform/latest/storage/understanding-persistent-storage.html).

## PVC Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | - | Name of an existing PVC to use (Bring Your Own PVC) |
| `size` | quantity | Component-specific | Requested storage size (required when operator creates PVC) |
| `retain` | boolean | `true` | Retain PVC when CR is deleted (immutable) |
| `storageClass` | string | - | Storage class name (immutable when `name` not specified) |
| `accessModes` | array | `[ReadWriteOnce]` | PVC access modes (immutable when `name` not specified) |

### Understanding the `name` Field

The `name` field controls how the operator handles PVC provisioning:

**When `name` is specified (Bring Your Own PVC)**:
- The operator uses the specified PVC name directly
- The operator does **not** create or manage the PVC
- You are responsible for creating and configuring the PVC beforehand
- Other PVC fields (`size`, `storageClass`, `accessModes`) are ignored

**When `name` is not specified (Operator-managed PVC)**:
- The operator generates a default PVC name (e.g., `tuf`, `rekor-<instance-name>-pvc`)
- If a PVC with the default name already exists, the operator discovers and uses it
- If no PVC exists, the operator creates one using the specified `size`, `storageClass`, and `accessModes`
- The `retain` field controls whether the PVC is deleted when the CR is deleted

### Example: Bring Your Own PVC

Use this approach when you need full control over PVC configuration or want to use a pre-provisioned volume:

1. Create the PVC manually:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-tuf-storage
  namespace: trusted-artifact-signer
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 100Mi
  storageClassName: ocs-storagecluster-cephfs
```

2. Reference the PVC in your RHTAS configuration:

```yaml
spec:
  tuf:
    replicas: 3
    pvc:
      name: "my-tuf-storage"  # Operator uses this existing PVC
```

### Example: Operator-managed PVC

Let the operator create and manage the PVC:

```yaml
spec:
  tuf:
    replicas: 3
    pvc:
      size: "100Mi"
      accessModes:
        - ReadWriteMany
      storageClass: "ocs-storagecluster-cephfs"
      retain: true  # PVC persists after CR deletion
```

## Alternatives to RWX Storage

If RWX storage is not available in your environment, consider these alternatives:

### For Rekor Attestations

Use cloud object storage (S3, GCS, Azure Blob) instead of file-based storage. This eliminates the RWX requirement for Rekor.

See [Configuring Rekor Attestation Storage](./rekor-attestation-storage.md) for detailed configuration options.

### For TUF

TUF requires RWX storage for HA deployments with multiple replicas. If RWX storage is not available in your environment, the only alternative is to run TUF with a single replica.

> **Note**: Running TUF with a single replica is not recommended for production environments as it creates a single point of failure.

## Limitations

- Not all storage classes support RWX. Verify your storage class capabilities before deployment.
- Once set, `accessModes` and `storageClass` cannot be changed without specifying a custom PVC name.

## Related Documentation

* [High Availability Overview](./high-availability-overview.md)
* [Configuring Rekor Attestation Storage](./rekor-attestation-storage.md)
* [Configuring External Database](./external-database.md)
* [Configuring External Search Index](./external-search-index.md)
