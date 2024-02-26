#!/bin/bash
BASE_DOMAIN=apps.$(oc get dns cluster -o jsonpath='{ .spec.baseDomain }')
OIDC_ISSUER=https://keycloak-keycloak-system.$BASE_DOMAIN/auth/realms/sigstore
sed -i "s|https://your-oidc-issuer-url|$OIDC_ISSUER|g" config/samples/rhtas_v1alpha1_securesign.yaml
sed -i 's|ClientID: "trusted-artifact-signer"|ClientID: "sigstore"|g' config/samples/rhtas_v1alpha1_securesign.yaml
oc create ns securesign
oc apply -f config/samples/rhtas_v1alpha1_securesign.yaml -n securesign
timeout 300 bash -c 'for i in trillian-db trillian-logserver trillian-logsigner fulcio-server; do until [ ! -z "$(oc get deployment $i -n securesign 2>/dev/null)" ]; do echo "Waiting for $i deployment to be created. Pods in securesign namespace:"; oc get pods -n securesign; sleep 3; done; done'
oc wait --for=condition=available deployment/trillian-db -n securesign --timeout=60s
oc wait --for=condition=available deployment/trillian-logserver -n securesign --timeout=60s
oc wait --for=condition=available deployment/trillian-logsigner -n securesign --timeout=60s
oc wait --for=condition=available deployment/fulcio-server -n securesign --timeout=60s
oc set env -n securesign deployment/fulcio-server SSL_CERT_DIR=/var/run/fulcio
timeout 300 bash -c 'for i in tuf ctlog rekor-redis rekor-server; do until [ ! -z "$(oc get deployment $i -n securesign 2>/dev/null)" ]; do echo "Waiting for $i deployment to be created. Pods in securesign namespace:"; oc get pods -n securesign; sleep 3; done; done'
oc wait --for=condition=available deployment/tuf -n securesign --timeout=60s
oc wait --for=condition=available deployment/ctlog -n securesign --timeout=60s
oc wait --for=condition=available deployment/rekor-redis -n securesign --timeout=60s
oc wait --for=condition=available deployment/rekor-server -n securesign --timeout=60s
curl -o /tmp/job.yaml https://raw.githubusercontent.com/cooktheryan/hello-world/main/job.yaml
sed -i "s|BASE_DOMAIN|$BASE_DOMAIN|g" /tmp/job.yaml
oc apply -f /tmp/job.yaml -n default
oc wait --for=condition=complete job/tas-test-sign-verify --timeout=5m -n default
