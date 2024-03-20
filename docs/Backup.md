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
    velero.io/storage-location: <StorageLocation>
  namespace: openshift-adp
spec:
  hooks: {}
  includedNamespaces:
  - openshift-rhtas-operator
  - trusted-artifact-signer
  includedResources: [] 
  excludedResources: VolumeSnapshots
  storageLocation: default
  ttl: 720h0m0s
EOF

oc apply -f BackupCr.yaml
```
Velero will then create a backup and store it within the storage device that was specified during the install process. When the OADP Operator is installed using the same storage medium on a new cluster the backup cr will created and available for use in the restore process. By default all resources within the specified namespaces are included. You can specify what to include within the included/excluded resources arrays within the above sample.

