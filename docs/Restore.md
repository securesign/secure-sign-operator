# Restore Procedures

## Prequisites
Performing a restore assumes the follwing.
- OADP operator is installed and configured correctly.
- You are utilising the same namespace structure as the backup.

## Same Cluster restore
If the cluster you are performing the restore action on is the same cluster as the original backup the following Restore Example should suffice.

```sh
cat << EOF > ./RestoreExample.yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: <BackName>
  namespace: openshift-adp
spec:
  backupName: <BackupName>
  includedResources: [] 
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
If on a new cluster it is advised to perform a restore only of the Persistent Volumes  as the configuration of the operator will have to be updated due to the new cluster. Before installing a new instance of the operator perform the following.

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
  excludedResources:
  - nodes
  - events
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
  - deployments
  - secrets
  - configmaps
  - cronjobs
  - replicasets
  - services
  - routes
  - ingresses
  - Securesign
  - pods
  - tuf
  - ctlog
  - rekor
  - fulcio
  - trillian
  - ConfigMap
  restorePVs: true 
  existingResourcePolicy: Update
EOF

oc apply -f ./RestoreExample.yaml
```