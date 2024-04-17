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

## Cluster restore
If the cluster you are performing the restore action on is the same cluster as the original backup the following Restore Example should suffice.

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