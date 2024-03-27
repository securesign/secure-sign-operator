# Backup Procedure
## Prerequisites
- Successfully installed and configured OADP Operator.
    - See OADP-Install.md.


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
  excludedResources: [VolumeSnapshots]
  storageLocation: <BackupStorageLocation>
  ttl: 720h0m0s
EOF

oc apply -f BackupCr.yaml
```
Velero will then create a backup and store it within the storage device that was specified during the install process. When the OADP Operator is installed using the same storage medium on a new cluster the backup cr will be created and available for use in the restore process. By default all resources within the specified namespaces are included. You can specify what to include within the included/excluded resources arrays within the above sample.

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
