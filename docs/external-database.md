# Configuring External Database for Trillian

This guide describes how to configure Trillian to use an external production-ready database in Red Hat Trusted Artifact Signer (RHTAS). Using an external database is recommended for production deployments instead of the operator-managed database.

## Overview

Trillian is the transparency log backend used by RHTAS components. It manages **two separate Merkle trees**: one for Rekor (artifact signing records) and one for CTlog (certificate transparency). Both trees are stored in the same database.

> **Note**: Trillian stores only the Merkle tree structure and leaf hashes for cryptographic verification. The actual Rekor entries and CTlog certificates are not stored in the database.

### Supported Database Backends

Trillian supports the following database backends:

* **Operator-managed MySQL** (default) - Deployed by the operator, suitable for development and testing only
* **External MySQL** - User-provided production-ready MySQL instance, including MySQL-compatible databases (MariaDB, AWS RDS MySQL, Azure Database for MySQL)
* **External PostgreSQL** - User-provided production-ready PostgreSQL instance (preview feature)

Database compatibility is based on the underlying Go drivers:
- MySQL: [github.com/go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)
- PostgreSQL: [github.com/jackc/pgx](https://github.com/jackc/pgx)

> **Important**: The operator-managed MySQL deployment is **not production-ready**. It runs as a single replica with a local PVC and lacks enterprise features such as high availability, automated backups, and disaster recovery. For production deployments, always use an external database.

## Database Configuration Fields

The `database` section in the Trillian specification includes the following fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `create` | boolean | `true` | When `true`, operator deploys managed MySQL. Set to `false` for external database. |
| `provider` | string | `mysql` | Database provider: `mysql` or `postgresql`. |
| `uri` | string | See below | Database connection URI with environment variable placeholders. |
| `pvc` | object | - | PVC configuration for managed database (only applies when `create: true`). |
| `tls` | object | - | TLS configuration for managed database (only applies when `create: true`). |

For external databases, use `spec.trustedCA` to configure TLS:

The `auth` section in the Trillian specification provides credentials:

| Field | Type | Description |
|-------|------|-------------|
| `auth.env` | array | Environment variables for authentication (referenced in URI template). |
| `auth.secretMount` | array | Secrets to mount as files (e.g., for TLS certificates). |

> **Note**: The `secretMount` configuration mounts secrets at `/var/run/secrets/tas/auth` by default.

> **Deprecation Notice**: The `databaseSecretRef` field is deprecated. Use `uri` with `auth.env` instead.

### URI Template Format

The `uri` field uses environment variable placeholders that are resolved from `auth.env`:

**MySQL format**:
```
$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)
```

**PostgreSQL format**:
```
$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)
```

## Using External MySQL

MySQL is the default and fully supported database backend for Trillian.

### Prerequisites

1. A production-ready MySQL instance (version 8.0 or later recommended)
2. Database created for Trillian usage
3. Database user with appropriate permissions
4. Database schema applied (provided by Trillian project)
5. Network connectivity from the OpenShift cluster to the MySQL instance

### Step 1: Prepare the Database

Connect to your MySQL instance and create the database and user:

```sql
CREATE DATABASE trillian;
CREATE USER 'trillian'@'%' IDENTIFIED BY 'your-secure-password';
GRANT ALL PRIVILEGES ON trillian.* TO 'trillian'@'%';
FLUSH PRIVILEGES;
```

Apply the Trillian schema:

```bash
# Download the schema
curl -o trillian-schema.sql https://raw.githubusercontent.com/securesign/trillian/main/storage/mysql/schema/storage.sql

# Apply the schema
mysql -h mysql.example.com -u trillian -p trillian < trillian-schema.sql
```

### Step 2: Create Database Credentials Secret

Create a secret containing the database credentials:

```bash
oc create secret generic trillian-db-credentials \
  --from-literal=mysql-host="mysql.example.com" \
  --from-literal=mysql-port="3306" \
  --from-literal=mysql-user="trillian" \
  --from-literal=mysql-password="your-secure-password" \
  --from-literal=mysql-database="trillian" \
  -n trusted-artifact-signer
```

### Step 3: Configure Trillian

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Trillian
metadata:
  name: trillian
  namespace: trusted-artifact-signer
spec:
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
```

### Configuration with TLS

For secure connections to an external MySQL database, use the `trustedCA` field to provide the CA certificate for server verification:

1. Create a ConfigMap with the CA certificate:

```bash
oc create configmap mysql-ca-bundle \
  --from-file=ca-bundle.crt=/path/to/ca-certificate.pem \
  -n trusted-artifact-signer
```

2. Configure Trillian with TLS using `trustedCA`:

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Trillian
metadata:
  name: trillian
  namespace: trusted-artifact-signer
spec:
  trustedCA:
    name: mysql-ca-bundle
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
```

## Using External PostgreSQL

PostgreSQL is an alternative database backend for Trillian.

### Prerequisites

1. A production-ready PostgreSQL instance (version 13 or later recommended)
2. Database created for Trillian usage
3. Database user with appropriate permissions
4. Database schema applied
5. Network connectivity from the OpenShift cluster to the PostgreSQL instance

### Step 1: Prepare the Database

Connect to your PostgreSQL instance and create the database and user:

```sql
CREATE DATABASE trillian;
CREATE USER trillian WITH PASSWORD 'your-secure-password';
GRANT ALL PRIVILEGES ON DATABASE trillian TO trillian;
```

Apply the Trillian schema from the [Trillian project](https://github.com/securesign/trillian/blob/main/storage/postgresql/schema/storage.sql).

### Step 2: Create Database Credentials Secret

Create a secret containing the database credentials:

```bash
oc create secret generic trillian-db-credentials \
  --from-literal=postgresql-host=your-postgresql-host \
  --from-literal=postgresql-port=5432 \
  --from-literal=postgresql-user=trillian \
  --from-literal=postgresql-password=your-secure-password \
  --from-literal=postgresql-database=trillian \
  -n trusted-artifact-signer
```

### Step 3: Configure Trillian

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Trillian
metadata:
  name: trillian
  namespace: trusted-artifact-signer
spec:
  database:
    create: false
    provider: postgresql
    uri: "$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)"
  auth:
    env:
      - name: DB_HOST
        valueFrom:
          secretKeyRef:
            name: trillian-db-credentials
            key: postgresql-host
      - name: DB_PORT
        valueFrom:
          secretKeyRef:
            name: trillian-db-credentials
            key: postgresql-port
      - name: DB_USER
        valueFrom:
          secretKeyRef:
            name: trillian-db-credentials
            key: postgresql-user
      - name: DB_PASSWORD
        valueFrom:
          secretKeyRef:
            name: trillian-db-credentials
            key: postgresql-password
      - name: DB_NAME
        valueFrom:
          secretKeyRef:
            name: trillian-db-credentials
            key: postgresql-database
```

## Database Secret Format

The database credentials secret should contain the connection details. The key names can be customized as they are mapped through `auth.env`:

**MySQL example**:

| Key | Description | Example |
|-----|-------------|---------|
| `mysql-host` | Database hostname | `mysql.example.com` |
| `mysql-port` | Database port | `3306` |
| `mysql-user` | Database username | `trillian` |
| `mysql-password` | Database password | `secure-password` |
| `mysql-database` | Database name | `trillian` |

**PostgreSQL example**:

| Key | Description | Example |
|-----|-------------|---------|
| `postgresql-host` | Database hostname | `postgresql.example.com` |
| `postgresql-port` | Database port | `5432` |
| `postgresql-user` | Database username | `trillian` |
| `postgresql-password` | Database password | `secure-password` |
| `postgresql-database` | Database name | `trillian` |

> **Note**: The key names in the secret can be customized. The mapping between secret keys and environment variables is defined in the `auth.env` section of the Trillian configuration.

## Limitations

- The `create` field cannot be changed after initial deployment. Migrating from managed to external database requires manual data migration.
- When `create: false`, the `uri` and `auth` fields must be configured.
- Users are responsible for database schema migrations, backup, and recovery for external databases.

## Related Documentation

* [High Availability Overview](./high-availability-overview.md)
* [Configuring External Search Index](./external-search-index.md)
* [Configuring RWX Storage](./pvc-rwx-storage.md)
