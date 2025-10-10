#!/bin/bash

# Delete TLS secrets (will be recreated by operator) and restart deployments in correct order.
# Switch to the namespace you want to delete the TLS secrets from.

echo "Deleting TLS secrets..."
oc delete secret securesign-sample-rekor-redis-tls --ignore-not-found=true
oc delete secret securesign-sample-ctlog-tls --ignore-not-found=true
oc delete secret securesign-sample-trillian-logserver-tls --ignore-not-found=true
oc delete secret securesign-sample-trillian-logsigner-tls --ignore-not-found=true
oc delete secret securesign-sample-trillian-db-tls --ignore-not-found=true

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
