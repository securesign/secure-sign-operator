# Certificate Transparency (CT) Log Signer Key Rotation and Sharding

This document provides a step-by-step guide for performing signer key rotation and sharding of the Certificate Transparency (CT) log in a Kubernetes-based environment. The procedure ensures the log remains functional, secure, and compliant with operational requirements during the process.

## Prerequisites

Before starting, ensure that:
- You have access to a Kubernetes cluster where the CT log is running.
- You have administrative permissions to modify the CT log configuration, scale deployments, create secrets, and patch resources.
- You have the `openssl` tool installed for key generation.

## Key Rotation Steps

### 1. Connect to Kubernetes Cluster

Set your context to the namespace that contains the CT log service:

```bash
kubectl config set-context --current --namespace=<namespace-name>
```

### 2. Backup the Current CT Log Configuration

Before making any changes, store the current CT log configuration and related keys for backup:

```bash
SERVER_CONFIG_NAME=$(kubectl get ctlog -o jsonpath='{.items[0].status.serverConfigRef.name}')
kubectl get secret $SERVER_CONFIG_NAME -o jsonpath="{.data.config}" | base64 --decode > config.txtpb
kubectl get secret $SERVER_CONFIG_NAME -o jsonpath="{.data.fulcio-0}" | base64 --decode > fulcio-0.pem
kubectl get secret $SERVER_CONFIG_NAME -o jsonpath="{.data.private}" | base64 --decode > private.pem
kubectl get secret $SERVER_CONFIG_NAME -o jsonpath="{.data.public}" | base64 --decode > public.pem
```

This backup will be needed for generating a new configuration.

### 3. Record the Current Tree ID

Store the `treeID` of the currently active CT log shard:

```bash
CURRENT_TREE_ID=$(kubectl get ctlog -o jsonpath='{.items[0].status.treeID}')
```

### 4. Drain the CT Log

Stop new entries from being added by setting the current log to a `DRAINING` state. This will prevent new entries but allow already submitted entries to be processed:

```bash
kubectl run --image registry.redhat.io/rhtas/updatetree-rhel9:latest --restart=Never --attach=true --rm=true -q -- updatetree --admin_server=trillian-logserver:8091 --tree_id=${CURRENT_TREE_ID} --tree_state=DRAINING
```

### 5. Monitor Queue Draining

It's critical to ensure that all pending entries in the log queue are processed before proceeding. Follow the instructions in [Trillian's documentation on freezing a log](https://github.com/google/trillian/blob/master/docs/howto/freeze_a_ct_log.md#monitor-queue--integration) to monitor the queue.

### 6. Freeze the CT Log

Once the queue has fully drained, freeze the log by setting the log state to `FROZEN`:

```bash
kubectl run --image registry.redhat.io/rhtas/updatetree-rhel9:latest --restart=Never --attach=true --rm=true -q -- updatetree --admin_server=trillian-logserver:8091 --tree_id=${CURRENT_TREE_ID} --tree_state=FROZEN
```

### 7. Create a New Merkle Tree

Now, create a new Merkle Tree that will serve as the new active shard:

```bash
NEW_TREE_ID=$(kubectl run createtree --image registry.redhat.io/rhtas/createtree-rhel9:latest --restart=Never --attach=true --rm=true -q -- -logtostderr=false --admin_server=trillian-logserver:8091 --display_name=ctlog-tree)
```

### 8. Generate New Private Key

Generate a new private key for the new CT log shard using OpenSSL:

```bash
openssl ecparam -genkey -name prime256v1 -noout -out new-ctlog.pem
openssl ec -in new-ctlog.pem -pubout -out new-ctlog-public.pem
openssl ec -in new-ctlog.pem -out new-ctlog.pass.pem -des3 -passout pass:"changeit"
```

### 9. Update the CT Log Configuration

You will now modify the old configuration stored in `config.txtpb` to:
- Add a `not_after_limit` field to the frozen log entry.
- Rename the `prefix` of the frozen log to clearly differentiate it from the new active log (e.g., `trusted-artifact-signer-0`).
- Define a new log configuration for the newly created tree, which includes adding a `not_after_start` field for the new log and using the new private key and tree ID.

#### **CT Log Configuration Format**
The CT Log configuration is written in **Protocol Buffer Text Format** (protobuf text format). The schema for this configuration can be found in the following repository: [CTFE Configuration Schema](https://github.com/securesign/certificate-transparency-go/blob/master/trillian/ctfe/configpb/config.proto). You will be editing this configuration file to accommodate the changes required for the frozen and new logs.

#### **Important Note on Timestamps:**
- The `not_after_limit` timestamp for the frozen log defines the end of the range of acceptable `NotAfter` values for certificates, which is exclusive. This means no certificates with a `NotAfter` date beyond this timestamp will be accepted for inclusion in this log.
- The `not_after_start` timestamp for the new log defines the beginning of the range of acceptable `NotAfter` values, inclusive.

#### **Tip**:
You can retrieve the current time values for `seconds` and `nanos` with the following commands: `date +%s`, `date +%N`

#### Example configuration (`config.txtpb`)

Below is an example of how the final configuration file should look, incorporating the frozen log and the new active log:

```prototext
backends:{backend:{name:"trillian" backend_spec:"trillian-logserver.test.svc:8091"}}
log_configs:{
  # frozen log
  config:{
    log_id:4836235718074713264
    prefix:"trusted-artifact-signer-0"
    roots_pem_file:"/ctfe-keys/fulcio-0"
    private_key:{[type.googleapis.com/keyspb.PEMKeyFile]:{path:"/ctfe-keys/private-0" password:"changeit"}}
    ext_key_usages:"CodeSigning"
    not_after_limit:{seconds:1713201754 nanos:155663000}
    log_backend_name:"trillian"
  }
  # new active log
  config:{ 
    log_id:1066448025935121985
    prefix:"trusted-artifact-signer"
    roots_pem_file:"/ctfe-keys/fulcio-0"
    private_key:{[type.googleapis.com/keyspb.PEMKeyFile]:{path:"/ctfe-keys/private" password:"changeit"}}
    ext_key_usages:"CodeSigning"
    not_after_start:{seconds:1713201754 nanos:155663000}
    log_backend_name:"trillian"
  }
}
```

In this configuration:
- The `frozen log` (identified by `CURRENT_TREE_ID`) has the `prefix` renamed to `trusted-artifact-signer-0`, and it includes a `not_after_limit` timestamp to stop accepting certificates with a `NotAfter` date beyond this point.
- The `new active log` (identified by `NEW_TREE_ID`) is set up with a new prefix (`trusted-artifact-signer`), a new private key, and includes a `not_after_start` timestamp, marking when the log will start accepting certificates.

### 10. Create a new Kubernetes secret

Store the new configuration and keys in a Kubernetes secret:

```bash
kubectl create secret generic ctlog-config \
   --from-file=config=config.txtpb \
   --from-file=private=new-ctlog.pass.pem \
   --from-file=public=new-ctlog-public.pem \
   --from-file=fulcio-0=fulcio-0.pem \
   --from-file=private-0=private.pem \
   --from-file=public-0=public.pem \
   --from-literal=password=changeit
```

### 11. Update Securesign resource

Patch the Securesign resource to use the new configuration and keys:

```bash
read -r -d '' SECURESIGN_PATCH <<EOF
[
    {
        "op": "replace",
        "path": "/spec/ctlog/serverConfigRef",
        "value": {"name": "ctlog-config"}
    },
    {
        "op": "replace",
        "path": "/spec/ctlog/privateKeyRef",
        "value": {"name": "ctlog-config", "key": "private"}
    },
    {
        "op": "replace",
        "path": "/spec/ctlog/privateKeyPasswordRef",
        "value": {"name": "ctlog-config", "key": "password"}
    },
    {
        "op": "replace",
        "path": "/spec/ctlog/publicKeyRef",
        "value": {"name": "ctlog-config", "key": "public"}
    }
]
EOF
kubectl patch securesign securesign-sample --type='json' -p="$SECURESIGN_PATCH"
```

### 12. Wait for CT Log server redeployment

Monitor the Kubernetes deployment to ensure the CT log server is redeployed with the updated configuration.

```bash
kubectl get pods -w -l app.kubernetes.io/name=ctlog
```

### 13. Update TUF Service

Follow the [TUF key rotation documentation](TODO) to add the new public key into TUF service.
