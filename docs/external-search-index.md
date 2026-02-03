# Configuring External Search Index for Rekor

This guide describes how to configure Rekor to use an external Redis instance as its search index backend. Using an external production-ready Redis instance is recommended for production deployments instead of the operator-managed Redis.

## Overview

Rekor uses a search index to enable efficient querying of entries in the transparency log. The search index supports:

* **Operator-managed Redis** (default) - Deployed by the operator, suitable for development and testing only
* **External Redis** - User-provided production-ready Redis instance

> **Important**: The operator-managed Redis deployment is **not production-ready**. It runs as a single replica without persistence guarantees, high availability, or backup capabilities. For production deployments, always use an external Redis instance.

## Search Index Configuration Fields

The `searchIndex` section in the Rekor specification includes the following fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `create` | boolean | `true` | When `true`, operator deploys managed Redis. Set to `false` for external Redis. |
| `provider` | string | - | Database provider. Required when `create: false`. Supported values: `redis`, `mysql` |
| `url` | string | - | Connection URL. Required when `create: false`. |
| `tls` | object | - | TLS configuration for the managed Redis (only applies when `create: true`). |

## Using External Redis

Rekor communicates with Redis using the [go-redis/v9](https://github.com/redis/go-redis) driver. Supported Redis versions and Redis-compatible implementations depend on this driver's compatibility.

### Prerequisites

1. A production-ready Redis instance
2. Network connectivity from the OpenShift cluster to the Redis instance
3. Redis credentials (if authentication is enabled)

### Redis URL Format

The Redis URL follows the standard Redis URI scheme:

```
redis://[username:password@]host:port[/database]
```

Examples:
* `redis://redis.example.com:6379` - No authentication
* `redis://:$(REDIS_PASSWORD)@redis.example.com:6379` - Password from environment variable
* `redis://$(REDIS_USER):$(REDIS_PASSWORD)@redis.example.com:6379` - Full authentication from environment variables
* `redis://redis.example.com:6379/0` - Specific database
* `rediss://redis.example.com:6379` - Redis with TLS (note the extra 's')

### Step 1: Create Redis Credentials Secret

If your Redis instance requires authentication, create a secret containing the credentials:

```bash
oc create secret generic redis-credentials \
  --from-literal=password=your-secure-password \
  -n trusted-artifact-signer
```

### Step 2: Configure Rekor

**Basic Configuration (No Authentication)**:

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  searchIndex:
    create: false
    provider: redis
    url: "redis://redis.example.com:6379"
```

**Configuration with Authentication**:

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  searchIndex:
    create: false
    provider: redis
    url: "redis://:$(REDIS_PASSWORD)@redis.example.com:6379"
  auth:
    env:
      - name: REDIS_PASSWORD
        valueFrom:
          secretKeyRef:
            name: redis-credentials
            key: password
```

### Configuration with TLS

For Redis instances with TLS enabled, use the `rediss://` scheme:

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  searchIndex:
    create: false
    provider: redis
    url: "rediss://:$(REDIS_PASSWORD)@redis.example.com:6379"
  auth:
    env:
      - name: REDIS_PASSWORD
        valueFrom:
          secretKeyRef:
            name: redis-credentials
            key: password
  trustedCA:
    name: redis-ca-bundle  # ConfigMap containing CA certificate
```

## Cloud Provider Compatibility

The external Redis configuration works with managed Redis services from major cloud providers:

- Amazon ElastiCache for Redis
- Azure Cache for Redis
- Google Cloud Memorystore for Redis

Use the same configuration as shown in the [Using External Redis](#using-external-redis) section. For TLS connections, use the `rediss://` scheme and reference the cloud provider's CA certificate via `trustedCA`.

## BackFillRedis CronJob

The Rekor deployment includes a BackFillRedis CronJob that ensures the search index stays synchronized with the transparency log. This job runs periodically to backfill any missing entries.

### Configuration

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  backFillRedis:
    enabled: true
    schedule: "0 0 * * *"  # Daily at midnight
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `true` | Enables the BackFillRedis CronJob. Cannot be disabled once enabled. |
| `schedule` | string | `0 0 * * *` | Cron schedule expression |

## MySQL as Search Index Provider (Tech Preview)

> **Tech Preview**: MySQL support for the search index is a Tech Preview feature. It is not recommended for production use.

Rekor also supports MySQL as a search index provider. This can be useful if you prefer to use a single database technology for your deployment.

> **Note**: Rekor automatically creates the required `EntryIndex` table on startup. No manual schema setup is required.

### Configuration

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Rekor
metadata:
  name: rekor
  namespace: trusted-artifact-signer
spec:
  searchIndex:
    create: false
    provider: mysql
    url: "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)"
  auth:
    env:
      - name: MYSQL_HOST
        valueFrom:
          secretKeyRef:
            name: rekor-mysql-credentials
            key: host
      - name: MYSQL_PORT
        valueFrom:
          secretKeyRef:
            name: rekor-mysql-credentials
            key: port
      - name: MYSQL_USER
        valueFrom:
          secretKeyRef:
            name: rekor-mysql-credentials
            key: user
      - name: MYSQL_PASSWORD
        valueFrom:
          secretKeyRef:
            name: rekor-mysql-credentials
            key: password
      - name: MYSQL_DATABASE
        valueFrom:
          secretKeyRef:
            name: rekor-mysql-credentials
            key: database
```

> **Note**: Redis is recommended over MySQL for the search index due to better performance characteristics for key-value lookups and range queries commonly used by Rekor.

## Limitations

- The `create` field cannot be changed after initial deployment. Migrating from managed to external Redis requires manual intervention.
- When `create: false`, the `provider` and `url` fields must be specified.
- Once the BackFillRedis CronJob is enabled, it cannot be disabled.
- External Redis requires network connectivity from all OpenShift nodes.

## Related Documentation

* [High Availability Overview](./high-availability-overview.md)
* [Configuring Rekor Attestation Storage](./rekor-attestation-storage.md)
* [Configuring External Database](./external-database.md)
* [Configuring RWX Storage](./pvc-rwx-storage.md)
