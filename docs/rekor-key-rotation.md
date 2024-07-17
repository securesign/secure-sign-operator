# Rotating the Signer Key for Rekor Service

This document provides detailed steps on how to rotate the signer key for the Rekor service. The process involves
sharding the Rekor log and then updating the signer key.

## Prerequisites

Before you begin, ensure you have the necessary access to your Kubernetes cluster and the Rekor CLI.

## Part 1: Freezing the Current Tree

In order to rotate the signer key effectively, it's crucial to transition the current tree into a frozen state, ensuring
it's only accessible for reading purposes. Simultaneously, a new tree needs to be created to serve as the active tree
for signing new records with the updated key. This process is facilitated through the sharding feature of Rekor, which
allows the log to be divided into multiple manageable parts. By following the initial 8 steps outlined in the
[Sharding the Rekor Log documentation](rekor-sharding.md), you will freeze the current log tree and establish a new log
tree ready for operations.

## Part 2: Rotating the Signer Key

Before proceeding with the rotation of the signer key, it's essential to complete **Part 1** to ensure the Rekor service
is prepared with a frozen current tree and a newly established active tree for continued operations with the updated
key. Once Part 1 is completed, ensure you have the following environment variables set:

```bash
CURRENT_SHARD_LENGTH=<length_of_frozen_shard>
CURRENT_TREE_ID=<frozen_tree_id>
NEW_TREE_ID=<new_tree_id>
CURRENT_SHARD_PUBLIC_KEY=<public_key_of_frozen_shard>
```

These variables are necessary for the subsequent steps to successfully rotate the signer key.

1. **Create New Private Key:** 
   Generate a new private key and store it in a Kubernetes secret. You can use the following commands:

   ```bash
   openssl ecparam -genkey -name secp384r1 -noout -out rekor.pem
   kubectl create secret generic rekor-signer-key --from-file=private=rekor.pem
   ```

1. **Update Securesign Resource:**

   Patch the Securesign resource to use the newly created secret, update the tree ID, and configure the sharding details for frozen log.

   ```bash
   read -r -d '' SECURESIGN_PATCH <<EOF
   [
       {
           "op": "replace",
           "path": "/spec/rekor/treeID",
           "value": "$NEW_TREE_ID"
       },
       {
           "op": "add",
           "path": "/spec/rekor/sharding",
           "value": {
               "treeID": "$CURRENT_TREE_ID",
               "treeLength": "$CURRENT_SHARD_LENGTH",
               "encodedPublicKey": "$CURRENT_SHARD_PUBLIC_KEY"
           }
       },
       {
           "op": "replace",
           "path": "/spec/rekor/signer/keyRef",
           "value": {"name": "rekor-signer-key", "key": "private"}
       },
       {
           "op": "remove",
           "path": "/spec/rekor/signer/keyPasswordRef"
       },
   ]
   EOF
   kubectl patch securesign securesign-sample --type='json' -p="$SECURESIGN_PATCH"
   ```

3. **Wait for Rekor Server Redeployment:**

   Monitor the Kubernetes deployment to ensure the Rekor server is redeployed with the updated configuration.

   ```bash
   kubectl get pods -w -l app.kubernetes.io/name=rekor-server
   ```

4. **Retrieve New Rekor Public Key:**

   After the Rekor server is redeployed, retrieve the new public key of the signer.

   ```bash
   NEW_SHARD_PUBLIC_KEY=$(curl $(oc get rekor -o jsonpath='{.items[0].status.url}')/api/v1/log/publicKey | base64)
   ```

5. **Update TUF Service:**

   Follow the [TUF key rotation documentation](TODO) to add the new public key into TUF service.
