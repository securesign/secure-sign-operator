# Sharding the Rekor Log

This document covers what Rekor log sharding is and how to shard the log for the Red Hat Trusted Artifact Signer (RHTAS) manged by Operator.

## What is sharding?

When Rekor is started for the first time, its backend is a transparency log built on a single [Merkle Tree](https://en.wikipedia.org/wiki/Merkle_tree).
This log can grow indefinitely as entries are added, which can present issues over time.
To resolve some of these issues the log can be "sharded" into multiple Merkle Trees.

## Why do we shard the log?

Sharding the log allows for:

* Freezing the current log and rotating signing keys if needed
* Easier and faster querying for entries from the tree
* Easier scaling and platform migrations

## How does this impact user experience?

End users shouldn't notice any difference in their experience.
They can still query via UUID, and Rekor will find the correct entry from whichever shard it's in.
Querying by log index works as well, since log indices are distinct and increase across shards.

## How do I shard the Rekor log?

**Sharding the Rekor log will require some downtime in your Rekor service.**
This is necessary because you'll need the length of the current shard later on, so new entries can't be added while sharding is in progress.

Follow these steps to shard the log:

1. Connect to your Kubernetes cluster and switch to the namespace that contains the running RHTAS stack:

   ```bash
   kubectl config set-context --current --namespace=<namespace-name>
   ```

1. Store the tree ID of the current active shard:

   ```bash
   CURRENT_TREE_ID=$(rekor-cli loginfo --format json | jq -r .TreeID)
   ```

1. Stop all traffic to Rekor so new entries can't be added by setting the log tree to a `DRAINING` state.

   ```bash
   kubectl run --image registry.redhat.io/rhtas/updatetree-rhel9:latest --restart=Never --attach=true --rm=true -q -- updatetree --admin_server=trillian-logserver:8091 --tree_id=${CURRENT_TREE_ID} --tree_state=DRAINING
   ```

   At this point, the log will not accept new entries, but there may be some that have already been submitted but not yet integrated.

1. Wait for the queue to drain.

   You can monitor the queue using the steps outlined in [Trillian's documentation](https://github.com/google/trillian/blob/master/docs/howto/freeze_a_ct_log.md#monitor-queue--integration). Essentially, you need to ensure that all pending entries have been processed before moving on to the next step. This is crucial to avoid data inconsistencies.

1. Set the log tree to a frozen state:

   **Warning**: Be sure to have completed the queue monitoring process set out in the previous section. If there are still queued leaves that have not been integrated, then setting the tree to frozen will put the log on a path to exceeding its MMD (Maximum Merge Delay).

   ```bash
   kubectl run --image registry.redhat.io/rhtas/updatetree-rhel9:latest --restart=Never --attach=true --rm=true -q -- updatetree --admin_server=trillian-logserver:8091 --tree_id=${CURRENT_TREE_ID} --tree_state=FROZEN
   ```

1. Store the length of the frozen tree:

   ```bash
   CURRENT_SHARD_LENGTH=$(rekor-cli loginfo --format json | jq -r .ActiveTreeSize)
   ```

1. Store the public key of the signer key used for the current active shard:

   ```bash
   CURRENT_SHARD_PUBLIC_KEY=$(curl -s $REKOR_URL/api/v1/log/publicKey | base64 | tr -d '\n')
   ```

1. Create a new Merkle Tree which will become the new active shard:

   ```bash
   NEW_TREE_ID=$(kubectl run createtree --image registry.redhat.io/rhtas/createtree-rhel9:latest --restart=Never --attach=true --rm=true -q -- -logtostderr=false --admin_server=trillian-logserver:8091 --display_name=rekor-tree)
   ```

1. At this point, we should have two trees: one is frozen, and the second is a new that will be used as the active shard. Example of stored values:

   ```bash
   CURRENT_SHARD_LENGTH=10000000
   CURRENT_TREE_ID=1949066653115536561
   NEW_TREE_ID=6137945217374551746
   CURRENT_SHARD_PUBLIC_KEY=LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0=
   ```

1. Update the sharding configuration on the Rekor resource:

   ```bash
   read -r -d '' SECURESIGN_PATCH <<EOF
   [
       {
           "op": "replace",
           "path": "/spec/rekor/treeID",
           "value": $NEW_TREE_ID
       },
       {
           "op": "add",
           "path": "/spec/rekor/sharding/-",
           "value": {
               "treeID": $CURRENT_TREE_ID,
               "treeLength": $CURRENT_SHARD_LENGTH,
               "encodedPublicKey": "$CURRENT_SHARD_PUBLIC_KEY"
           }
       }
   ]
   EOF
   kubectl patch securesign securesign-sample --type='json' -p=$SECURESIGN_PATCH
   ```

1. Wait until the operator spins up the Rekor server with the new configuration.

1. Congratulations, you've successfully sharded the log!

## Testing the Sharding Process

Once you've completed the sharding process, it's important to ensure everything is working correctly. Here are some steps to verify the new setup:

1. **Check the Logs**: Monitor the logs of the Rekor server to ensure there are no errors related to the new shard. You can use the following command to stream the logs:

   ```bash
   kubectl logs -f deploy/rekor-server
   ```

1. **Verify Entries**: Submit new entries to the Rekor log and ensure they are being added to the new shard. You can use the `rekor-cli` to add entries and then query them to verify they are correctly stored.

   ```bash
   rekor-cli upload --artifact <path_to_artifact> --public-key <path_to_public_key>
   ```

1. **Query the Shard**: Use the `rekor-cli` to query the log by UUID and log index to ensure the entries are retrievable from the new shard.

   ```bash
   rekor-cli get --uuid <uuid>
   ```

By following these steps, you can confirm that the sharding process was successful and that your Rekor server is operating as expected with the new shard configuration.

---

Sources:
- [Sharding the Rekor Log](https://docs.sigstore.dev/logging/sharding/)
- [Freeze a ct log](https://github.com/google/trillian/blob/master/docs/howto/freeze_a_ct_log.md)
