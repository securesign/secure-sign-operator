# Restore Procedures

## Prequisites
Performing a restore assumes the follwing.
- OADP operator is installed and configured correctly.
- You are utilising the same namespace structure as the backup.
- THe operator is currently disabled or not present on the cluster.

## Disable operator
If the operator is installed an you wish to perform a restore please use the following command to scale down the operator deployment.

```sh
oc scale deploy rhtas-operator-controller-manager --replicas=0 -n openshift-operators
```

Once restore operations have completed you can reactivate the operator by scaling back up its deployment.

```sh
oc scale deploy rhtas-operator-controller-manager --replicas=1 -n openshift-operators
```

## Same Cluster restore
If the cluster you are performing the restore action on is the same cluster as the original backup the following Restore Example should suffice.

```sh
cat << EOF > ./RestoreExample.yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <BackupName>
  namespace: openshift-adp
spec:
  backupName: <BackupName>
  includedResources:
    - pvc
    - secrets
    - configmaps
    - securesign.rhtas.redhat.com
    - trillian.rhtas.redhat.com
    - ctlog.rhtas.redhat.com
    - fulcio.rhtas.redhat.com
    - rekor.rhtas.redhat.com
    - tuf.rhtas.redhat.com
  restoreStatus:
    includedResources:
      - securesign.rhtas.redhat.com
      - trillian.rhtas.redhat.com
      - ctlog.rhtas.redhat.com
      - fulcio.rhtas.redhat.com
      - rekor.rhtas.redhat.com
      - tuf.rhtas.redhat.com
  excludedResources:
  - nodes
  - events
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
  restorePVs: true 
  existingResourcePolicy: Update
EOF

oc apply -f ./RestoreExample.yaml
```

## New Cluster Restore
If on a new cluster it is advised to perform a restore only of the Persistent Volumes as the configuration of the operator will have to be updated due to the new cluster. Before installing a new instance of the operator perform the following. When updating the operators configuration be sure to specify the treeID's for ctlog and rekor from the backup or use the same rekor and ctlog Custom Resources from the backup.

```sh
cat << EOF > ./RestoreExample.yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <RestoreName>
  namespace: openshift-adp
spec:
  backupName: <BackupName>
  includedResources:
    - pvc
    - secrets
    - configmaps
    - securesign.rhtas.redhat.com
    - trillian.rhtas.redhat.com
    - ctlog.rhtas.redhat.com
    - fulcio.rhtas.redhat.com
    - rekor.rhtas.redhat.com
    - tuf.rhtas.redhat.com
  restoreStatus:
    includedResources:
      - securesign.rhtas.redhat.com
      - trillian.rhtas.redhat.com
      - ctlog.rhtas.redhat.com
      - fulcio.rhtas.redhat.com
      - rekor.rhtas.redhat.com
      - tuf.rhtas.redhat.com
  excludedResources:
  - nodes
  - events
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
  excludedResources:
  - nodes
  - events
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
  restorePVs: true 
  existingResourcePolicy: Update
EOF

oc apply -f ./RestoreExample.yaml
```