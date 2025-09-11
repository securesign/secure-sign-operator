# Copying Data from One PVC to Another
When enabling HA (v1.3.0+) for services like Rekor and TUF, you may need to migrate data from a ReadWriteOnce (RWO) PVC to a ReadWriteMany (RWX) PVC so you can scale replicas correctly.

Prerequisites
* Access to the Kubernetes/OpenShift cluster and the target namespace.
* Permissions to create PVCs, Pods, and Jobs.
* A source PVC (RWO) with existing data.
* A target PVC (RWX) large enough to hold the data.
* oc (or kubectl) installed and logged in.

1. Scale down resource

      Scale Resource to 0 replicas:
      ```sh
      oc patch securesign $SECURESIGN_INSTANCE \
        -n $NAMESPACE \
        --type=json \
        -p '[{"op": "replace", "path": "/spec/<resource_name>/replicas", "value": 0}]'
      ```

2. Set the PVC names
      ```sh
      export SOURCE_PVC=<source-pvc-name>
      export DEST_PVC=<destination-pvc-name>
      ```

3. Create the job below which should copy data from one pvc to another

      ```sh
      cat <<EOF | oc apply -f -
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: pvc-copy-rwo-to-rwx
        namespace: ${NAMESPACE}
      spec:
        backoffLimit: 1
        ttlSecondsAfterFinished: 600
        template:
          spec:
            restartPolicy: OnFailure
            containers:
            - name: pvc-copy
              image: registry.redhat.io/openshift4/ose-cli:latest
              command: ["/bin/bash","-lc"]
              args:
                - |
                  set -euo pipefail
                  echo "Copying from /src to /dest..."
                  rsync -rlH --no-perms --no-owner --no-group --numeric-ids /src/ /dest/
                  echo "Done."
              volumeMounts:
                - name: src
                  mountPath: /src
                  readOnly: true
                - name: dest
                  mountPath: /dest
            volumes:
              - name: src
                persistentVolumeClaim:
                  claimName: ${SOURCE_PVC}
                  readOnly: true
              - name: dest
                persistentVolumeClaim:
                  claimName: ${DEST_PVC}
      EOF
      ```

4. Update the PVC name and scale back up replicas

      Point Resource at the new PVC and bring replicas back up:
      ```
      oc patch securesign $SECURESIGN_INSTANCE \
        -n $NAMESPACE \
        --type=json \
        -p '[{"op": "replace", "path": "/spec/<resource_name>/pvc/name", "value": "'${DEST_PVC}'"}]'

      oc patch securesign $SECURESIGN_INSTANCE \
        -n $NAMESPACE \
        --type=json \
        -p '[{"op": "replace", "path": "/spec/<resource_name>/replicas", "value": 1}]'
      ```
