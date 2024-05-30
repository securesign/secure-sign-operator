# Backup Procedure
## Prerequisites
- Successfully installed and configured OADP Operator.
    - See OADP-Install.md.
    - Configure CSI backup


## Configure CSI VolumeStorageClass for Backup Procedure

```sh
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: <volume_snapshot_class_name>
  labels:
    velero.io/csi-volumesnapshot-class: "true" 
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: true 
driver: <csi_driver>
deletionPolicy: <deletion_policy_type> 
```

Add the velero label to your clusters  default snapshot class/ the snapshot class that is used for your Persistent Volumes



## Backup Process
To create a singular backup instance create a Backup CR from the following template or from the template provided in the samples directory.

```sh
cat << EOF > ./BackupCr.yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: <Backup-Name>
  labels:
    velero.io/storage-location: <BackupStorageLocation>
  namespace: openshift-adp
spec:
  hooks: {}
  includedNamespaces:
  - trusted-artifact-signer
  includedResources: [] 
  excludedResources: []
  snapshotMoveData: true
  storageLocation: <BackupStorageLocation>
  ttl: 720h0m0s
EOF

oc apply -f BackupCr.yaml
```
Velero will then create a backup and store it within the storage device that was specified during the install process. When the OADP Operator is installed using the same storage medium on a new cluster the backup cr will be created and available for use in the restore process. By default all resources within the specified namespaces are included. You can specify what to include within the included/excluded resources arrays within the above sample.

### Backup failure
If the backup is unsuccessful or 'Partially Failed' status scale down the Rekor-Server and Trillian-db deployments. Depending on the storage class being used the persistent volumes cannot be actively in-use otherwise the backup will not complete.


## Scheduled Backup
To schedule the backup process is almost identical process to creating the backup but with the added cron scheduling. The below example would create a backup at 7am everyday.

```sh
$ cat << EOF ./ScheduleBackupCr.yaml
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: <schedule>
  namespace: openshift-adp
spec:
  schedule: 0 7 * * * 
  template:
    hooks: {}
    includedNamespaces:
    - trusted-artifact-signer
    includedResources: [] 
    excludedResources: [VolumeSnapshots]
    storageLocation: <BackupStorageLocation>
    ttl: 720h0m0s
EOF

oc apply -f ScheduleBackupCr.yaml
```

## Extra Backup Info
For extra info and clarification see the OADP backing up section within the [OADP Docs](https://docs.openshift.com/container-platform/4.15/backup_and_restore/application_backup_and_restore/backing_up_and_restoring/backing-up-applications.html).


