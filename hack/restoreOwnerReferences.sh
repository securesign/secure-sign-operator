#!/bin/bash

# List of resources to check
RESOURCES=("Fulcio" "Rekor" "Trillian" "TimestampAuthority" "CTlog" "Tuf")


function validate_owner() {
    local RESOURCE=$1
    local ITEM=$2
    local OWNER_NAME=$3

    # Check all the labels exist and are the same
    LABELS=("app.kubernetes.io/instance" "app.kubernetes.io/part-of" "velero.io/backup-name" "velero.io/restore-name")
    for LABEL in "${LABELS[@]}"; do
        PARENT_LABEL=$(oc get Securesign "$OWNER_NAME" -o json | jq -r ".metadata.labels[\"$LABEL\"]")
        CHILD_LABEL=$(oc get $RESOURCE "$ITEM" -o json | jq -r ".metadata.labels[\"$LABEL\"]")

        if [[ -z "$CHILD_LABEL" || $CHILD_LABEL == "null" ]]; then
            echo "  $LABEL label missing in $RESOURCE"
            return 1
        elif [[ -z "$PARENT_LABEL" || $PARENT_LABEL == "null" ]]; then
            echo "  $LABEL label missing in Securesign"
            return 1
        elif [[ "$CHILD_LABEL" != "$PARENT_LABEL" ]]; then
            echo "  $LABEL labels not matching: $CHILD_LABEL != $PARENT_LABEL"
            return 1
        fi
    done

    return 0
}


for RESOURCE in "${RESOURCES[@]}"; do
    echo "Checking $RESOURCE ..."

    # Get all resources missing ownerReferences
    MISSING_REFS=$(oc get $RESOURCE -o json | jq -r '.items[] | select(.metadata.ownerReferences == null) | .metadata.name')

    for ITEM in $MISSING_REFS; do
        echo "  Missing ownerReferences in $RESOURCE/$ITEM"

        # Find the expected owner based on labels
        OWNER_NAME=$(oc get $RESOURCE "$ITEM" -o json | jq -r '.metadata.labels["app.kubernetes.io/name"]')

        if [[ -z "$OWNER_NAME" || "$OWNER_NAME" == "null" ]]; then
            echo "  Skipping $RESOURCE/$ITEM: name not found in labels"
            continue
        fi

        if ! validate_owner $RESOURCE $ITEM $OWNER_NAME; then
          echo "  Skipping ..."
          continue
        fi

        # Try to get the owner's UID from Securesign
        OWNER_UID=$(oc get Securesign "$OWNER_NAME" -o jsonpath='{.metadata.uid}' 2>/dev/null)

        if [[ -z "$OWNER_UID" || "$OWNER_UID" == "null" ]]; then
            echo "  Failed to find Securesign/$OWNER_NAME UID, skipping ..."
            continue
        fi

        echo "  Found owner: Securesign/$OWNER_NAME (UID: $OWNER_UID)"

        # Patch the object with the restored ownerReference
        oc patch $RESOURCE "$ITEM" --type='merge' -p "{
          \"metadata\": {
            \"ownerReferences\": [
              {
                \"apiVersion\": \"rhtas.redhat.com/v1alpha1\",
                \"kind\": \"Securesign\",
                \"name\": \"$OWNER_NAME\",
                \"uid\": \"$OWNER_UID\",
                \"controller\": true,
                \"blockOwnerDeletion\": true
              }
            ]
          }
        }"

        echo "Restored ownerReferences for $RESOURCE/$ITEM"
    done
done

echo "... done"
