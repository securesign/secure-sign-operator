# High Availability Deployment Guide

## Overview

This guide describes how to configure Red Hat Trusted Artifact Signer (RHTAS) for high availability (HA) deployments. A high availability deployment ensures that your RHTAS instance can continue operating even if individual components fail, providing improved reliability and uptime for production environments.

## Prerequisites

- An OpenShift cluster with at least 3 worker nodes
- Cluster administrator privileges
- Understanding of Kubernetes concepts including Pods, Deployments, PersistentVolumes, and StorageClasses
- Access to production-ready storage solutions (external databases, object storage, Redis)

## High Availability Architecture

A high availability RHTAS deployment includes:

- **Multiple replicas**: At least 3 replicas of each component distributed across different cluster nodes
- **Production-ready storage**: External databases and object storage solutions instead of operator-managed storage
- **Pod distribution**: Pod anti-affinity rules to ensure replicas run on different nodes
- **Shared storage**: ReadWriteMany (RWX) access mode for components requiring shared storage

## Component-Specific HA Requirements

### Rekor

Rekor requires the following for HA deployment:

- **Replicas**: Minimum 3 replicas
- **Pod distribution**: Pod anti-affinity to distribute replicas across nodes
- **Attestation storage**: Production-ready object storage (S3, GCS, Azure Blob) or RWX PVC
- **Search index**: Production-ready Redis instance (not operator-managed)
- **Trillian backend**: External database (see [External Database Configuration](./external-database.md))

**Important**: When using file-based attestation storage (`file://` protocol), the PVC must use `ReadWriteMany` access mode for deployments with more than 1 replica.

### TUF

TUF requires the following for HA deployment:

- **Replicas**: Minimum 3 replicas
- **Pod distribution**: Pod anti-affinity to distribute replicas across nodes
- **Storage**: PVC with `ReadWriteMany` access mode (required for replicas > 1)

**Important**: TUF deployments with more than 1 replica require `ReadWriteMany` access mode. Without RWX storage, only a single replica can be deployed.

### Fulcio

Fulcio requires the following for HA deployment:

- **Replicas**: Minimum 3 replicas
- **Pod distribution**: Pod anti-affinity to distribute replicas across nodes
- **Stateless**: Fulcio is stateless and does not require shared storage

### CTlog

CTlog requires the following for HA deployment:

- **Replicas**: Minimum 3 replicas
- **Pod distribution**: Pod anti-affinity to distribute replicas across nodes
- **Trillian backend**: External database (shared with Rekor)
- **Stateless**: CTlog is stateless and does not require shared storage

### Trillian

Trillian requires the following for HA deployment:

- **External database**: Production-ready MySQL or PostgreSQL database
- **LogServer replicas**: Minimum 3 replicas with pod anti-affinity
- **LogSigner replicas**: Minimum 3 replicas with pod anti-affinity

**Note**: The LogSigner uses **per-tree leader election** based on Tree ID. Each Merkle tree has its own independent leader election, so different LogSigner instances can be leaders for different trees simultaneously (e.g., one instance signs the Rekor tree while another signs the CTlog tree). If a leader fails, another instance is automatically elected for that specific tree.

### Timestamp Authority (TSA)

Timestamp Authority requires the following for HA deployment:

- **Replicas**: Minimum 3 replicas
- **Pod distribution**: Pod anti-affinity to distribute replicas across nodes
- **Stateless**: TSA is stateless and does not require shared storage

## Complete HA Configuration Example

The following example demonstrates a complete high availability configuration for RHTAS. For detailed configuration options, see:

* [Configuring External Database](./external-database.md) - Trillian database configuration
* [Configuring Rekor Attestation Storage](./rekor-attestation-storage.md) - Attestation storage options
* [Configuring External Search Index](./external-search-index.md) - Redis search index configuration
* [Configuring RWX Storage](./pvc-rwx-storage.md) - TUF storage requirements

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Securesign
metadata:
  name: securesign-ha
  namespace: trusted-artifact-signer
spec:
  trillian:
    database:
      create: false
      provider: mysql
      uri: "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)"
    auth:
      env:
        - name: MYSQL_HOST
          valueFrom:
            secretKeyRef:
              name: trillian-db-credentials
              key: mysql-host
        - name: MYSQL_PORT
          valueFrom:
            secretKeyRef:
              name: trillian-db-credentials
              key: mysql-port
        - name: MYSQL_USER
          valueFrom:
            secretKeyRef:
              name: trillian-db-credentials
              key: mysql-user
        - name: MYSQL_PASSWORD
          valueFrom:
            secretKeyRef:
              name: trillian-db-credentials
              key: mysql-password
        - name: MYSQL_DATABASE
          valueFrom:
            secretKeyRef:
              name: trillian-db-credentials
              key: mysql-database
    server:
      replicas: 3
      resources:
        requests:
          cpu: "500m"
          memory: "256Mi"
        limits:
          cpu: "1000m"
          memory: "512Mi"
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: app.kubernetes.io/name
                      operator: In
                      values:
                        - trillian-logserver
                topologyKey: kubernetes.io/hostname
    signer:
      replicas: 3
      resources:
        requests:
          cpu: "100m"
          memory: "128Mi"
        limits:
          cpu: "250m"
          memory: "256Mi"
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: app.kubernetes.io/name
                      operator: In
                      values:
                        - trillian-logsigner
                topologyKey: kubernetes.io/hostname

  rekor:
    replicas: 3
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "1000m"
        memory: "512Mi"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/name
                    operator: In
                    values:
                      - rekor-server
              topologyKey: kubernetes.io/hostname
    attestations:
      enabled: true
      url: "s3://my-attestation-bucket?region=us-east-1"
    searchIndex:
      create: false
      provider: redis
      url: "redis://redis.example.com:6379"
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

  fulcio:
    replicas: 3
    resources:
      requests:
        cpu: "250m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/name
                    operator: In
                    values:
                      - fulcio-server
              topologyKey: kubernetes.io/hostname
    config:
      OIDCIssuers:
        - ClientID: "trusted-artifact-signer"
          IssuerURL: "https://your-oidc-issuer.example.com"
          Issuer: "https://your-oidc-issuer.example.com"
          Type: "email"
    certificate:
      organizationName: "Example Organization"
      organizationEmail: "admin@example.com"

  ctlog:
    replicas: 3
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "250m"
        memory: "256Mi"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/name
                    operator: In
                    values:
                      - ctlog
              topologyKey: kubernetes.io/hostname

  tuf:
    replicas: 3
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "250m"
        memory: "256Mi"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/name
                    operator: In
                    values:
                      - tuf
              topologyKey: kubernetes.io/hostname
    pvc:
      accessModes:
        - ReadWriteMany
      size: "100Mi"
      storageClass: "ocs-storagecluster-cephfs"

  tsa:
    replicas: 3
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "250m"
        memory: "256Mi"
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app.kubernetes.io/name
                    operator: In
                    values:
                      - tsa-server
              topologyKey: kubernetes.io/hostname
    signer:
      certificateChain:
        rootCA:
          organizationName: "Example Root Organization"
          organizationEmail: "admin@example.com"
        intermediateCA:
          - organizationName: "Example Intermediate Organization"
            organizationEmail: "admin@example.com"
        leafCA:
          organizationName: "Example Leaf CA"
          organizationEmail: "admin@example.com"
```

## Pod Configuration Options

Most RHTAS components support the following pod configuration options for controlling deployment behavior:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `replicas` | integer | `1` | Number of desired pod replicas |
| `affinity` | object | - | Pod scheduling constraints (affinity/anti-affinity rules) |
| `resources` | object | - | CPU and memory requests/limits |
| `tolerations` | array | - | Tolerations for node taints |

### Replicas

The `replicas` field specifies the number of pod instances to run. For high availability, a minimum of 3 replicas is recommended:

```yaml
spec:
  rekor:
    replicas: 3
```

### Affinity

The `affinity` field controls pod scheduling constraints. For HA deployments, pod anti-affinity is used to distribute replicas across different nodes.

**Soft anti-affinity** (preferred, allows scheduling if constraints cannot be met):

```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - rekor-server
          topologyKey: kubernetes.io/hostname
```

**Hard anti-affinity** (required, pods will not schedule if constraints cannot be met):

```yaml
affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
            - key: app.kubernetes.io/name
              operator: In
              values:
                - rekor-server
        topologyKey: kubernetes.io/hostname
```

### Resources

The `resources` field defines CPU and memory requests and limits for pods. Setting appropriate resource constraints ensures predictable performance and prevents resource contention:

```yaml
spec:
  rekor:
    replicas: 3
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "2000m"
        memory: "2Gi"
```

> **Note**: Resource requirements vary based on workload. Monitor actual usage and adjust accordingly.

### Tolerations

The `tolerations` field allows pods to be scheduled on nodes with specific taints. This is useful when dedicating nodes for RHTAS workloads:

```yaml
spec:
  rekor:
    replicas: 3
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "rhtas"
        effect: "NoSchedule"
```

To use tolerations effectively, first taint the dedicated nodes:

```bash
oc adm taint nodes <node-name> dedicated=rhtas:NoSchedule
```

## Storage Considerations

### ReadWriteMany (RWX) Storage

For components requiring shared storage (TUF and Rekor with file-based attestation storage), ReadWriteMany (RWX) access mode is required when running multiple replicas.

See [Configuring RWX Storage](./pvc-rwx-storage.md) for detailed configuration options and storage class recommendations.

### Cloud Object Storage for Rekor

For Rekor attestation storage, using cloud object storage (S3, GCS, Azure Blob) eliminates the RWX requirement and is recommended for production deployments.

See [Rekor Attestation Storage Configuration](./rekor-attestation-storage.md) for detailed configuration options.

## Related Documentation

* [Configuring External Database](./external-database.md)
* [Configuring External Search Index](./external-search-index.md)
* [Configuring Rekor Attestation Storage](./rekor-attestation-storage.md)
* [Configuring RWX Storage](./pvc-rwx-storage.md)
