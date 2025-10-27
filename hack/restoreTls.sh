#!/bin/bash

# Delete TLS secrets (will be recreated by operator) and restart deployments in correct order.
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

echo "Restarting Trillian components ..."
oc rollout restart deployment trillian-db
oc rollout restart deployment trillian-logserver
oc rollout restart deployment trillian-logsigner

echo "Restarting Redis ..."
oc rollout restart deployment rekor-redis

echo "Restarting CTlog ..."
oc rollout restart deployment ctlog

echo "All deployments restarted. New TLS secrets:"
oc get secrets | grep tls
