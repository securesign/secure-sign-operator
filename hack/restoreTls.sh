#!/bin/bash

# Delete TLS secrets (will be recreated by operator).
# Switch to the namespace you want to delete the TLS secrets from.

# Discover the Securesign instance name
INSTANCE_NAME=$(oc get Securesign -o jsonpath='{.items[0].metadata.name}')

if [[ -z "$INSTANCE_NAME" ]]; then
    echo "No Securesign instance found in current namespace"
    exit 1
fi

echo "Deleting TLS secrets..."
oc delete secret ${INSTANCE_NAME}-rekor-redis-tls --ignore-not-found=true
oc delete secret ${INSTANCE_NAME}-ctlog-tls --ignore-not-found=true
oc delete secret ${INSTANCE_NAME}-trillian-logserver-tls --ignore-not-found=true
oc delete secret ${INSTANCE_NAME}-trillian-logsigner-tls --ignore-not-found=true
oc delete secret ${INSTANCE_NAME}-trillian-db-tls --ignore-not-found=true
