# Restore Procedures

## Prequisites
Performing a restore assumes the following:
- OADP operator is installed and configured correctly.
- You are utilising the same namespace structure as the backup.
- The operator is currently disabled, but installed on a cluster. Needs to be enabled short after restore process is started to claim persistent volumes. 

## Disable operator
If the operator is installed and you wish to perform a restore, use the following command to scale down the operator deployment.

```sh
oc scale deploy rhtas-controller-manager --replicas=0 -n openshift-operators
```

Once restore operations are running, you can reactivate the operator by scaling back up its deployment - without enabling the operator persistent volumes are not claimed.

```sh
oc scale deploy rhtas-controller-manager --replicas=1 -n openshift-operators
```

## Cluster restore
If the cluster you are performing the restore action on is the same cluster as the original backup, the following Restore Example should suffice.

```sh
cat << EOF > ./RestoreExample.yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <RestoreName>
  namespace: openshift-adp
spec:
  backupName: <BackupName>
  includedResources: []
  restoreStatus:
    includedResources:
      - securesign.rhtas.redhat.com
      - trillian.rhtas.redhat.com
      - ctlog.rhtas.redhat.com
      - fulcio.rhtas.redhat.com
      - rekor.rhtas.redhat.com
      - tuf.rhtas.redhat.com
      - timestampauthority.rhtas.redhat.com
  excludedResources:
  - pod
  - deployment
  - nodes
  - route
  - service
  - replicaset
  - events
  - cronjob
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
  - pods
  - deployments
  restorePVs: true 
  existingResourcePolicy: update
EOF

oc apply -f RestoreExample.yaml
```

If the restore is done on a different cluster, few more steps needs to be done. First, delete the secret for Trillian DB which will be recreated by operator,
and restart the pod:

```sh
oc delete secret securesign-sample-trillian-db-tls
oc delete pod trillian-db-xxx
```

After the restore process is finished and all the pods are running, run the [restoreOwnerReferences.sh](../hack/restoreOwnerReferences.sh) script to recreate
ownerReferences, which were lost on a new cluster, as the owner has new UID.

## Cross Provider Restore 
To perform a restore on a cluster using different storage classes create a yaml file based upon the following:

```sh
cat << EOF > ./changestorageclass.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: change-storage-class-config
  namespace: openshift-adp
  labels:
    velero.io/plugin-config: ""
    velero.io/change-storage-class: RestoreItemAction
data:
  gp3-csi: ssd-csi
EOF

oc apply -f changestorageclass.yaml
```

The above example is swapping from an Amazon Web Services storage solution to a Google Cloud Provider Solution.