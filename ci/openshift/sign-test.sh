#!/bin/bash
# If environment variable BASE_DOMAIN is not set look up the value using oc because it's most likely an OpenShift cluster
if [ -z "$DOMAIN" ]; then
  BASE_DOMAIN=apps.$(oc get dns cluster -o jsonpath='{ .spec.baseDomain }')
else
  BASE_DOMAIN=apps.$DOMAIN
fi
OIDC_ISSUER=https://keycloak-keycloak-system.$BASE_DOMAIN/auth/realms/trusted-artifact-signer
# If EKS is true, change the keycloak URL to the existing Keycloak URL
if [ "$EKS" = "true" ]; then
  OIDC_ISSUER=${{ TESTING_KEYCLOAK }}/auth/realms/trusted-artifact-signer
fi
sed -i "s|https://your-oidc-issuer-url|$OIDC_ISSUER|g" config/samples/rhtas_v1alpha1_securesign.yaml



oc create ns securesign
oc apply -f config/samples/rhtas_v1alpha1_securesign.yaml -n securesign
# If EKS is true, we need to create a pull secret using the registry credentials from /tmp
if [ "$EKS" = "true" ]; then
  oc create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/config.json --type=kubernetes.io/dockerconfigjson -n securesign
fi

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

cat <<EOF | sed "s/BASE_DOMAIN/${BASE_DOMAIN}/g" > job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: tas-test-sign-verify
  labels:
    app.kubernetes.io/component: trusted-artifact-signer
  annotations:
    helm.sh/hook: test
spec:
  template:
    spec:
      initContainers:
      - name: buildah
        image: quay.io/buildah/stable
        command: ["/bin/sh", "-c"]
        args:
        - |
            buildah pull alpine:latest
            buildah tag alpine:latest ttl.sh/sigstore-test:5m
            buildah push ttl.sh/sigstore-test:5m
        securityContext:
            privileged: true
      containers:
      - name: cosign
        image: quay.io/redhat-user-workloads/rhtas-tenant/cli-1-0-gamma/cosign-cli-2-2@sha256:151f4a1e721b644bafe47bf5bfb8844ff27b95ca098cc37f3f6cbedcda79a897
        env:
        - name: OIDC_AUTHENTICATION_REALM
          value: "trusted-artifact-signer"
        - name: FULCIO_URL
          value: "https://fulcio-server-securesign.${BASE_DOMAIN}"
        - name: OIDC_ISSUER_URL
          value: "${OIDC_ISSUER}"
        - name: REKOR_URL
          value: "https://rekor-server-securesign.${BASE_DOMAIN}"
        - name: TUF_URL
          value: "https://tuf-securesign.${BASE_DOMAIN}"
        command: ["/bin/sh", "-c"]
        args:
          - |
            trust anchor --store /run/secrets/kubernetes.io/serviceaccount/ca.crt
            cosign initialize --mirror=\$TUF_URL --root=\$TUF_URL/root.json
            TOKEN=\$(curl -X POST -H "Content-Type: application/x-www-form-urlencoded" -d "username=jdoe" -d "password=secure" -d "grant_type=password" -d "scope=openid" -d "client_id=trusted-artifact-signer" \$OIDC_ISSUER_URL/protocol/openid-connect/token |  sed -E 's/.*"access_token":"([^"]*).*/\1/')
            cosign sign -y --fulcio-url=\$FULCIO_URL --rekor-url=\$REKOR_URL --oidc-issuer=\$OIDC_ISSUER_URL --identity-token=\$TOKEN ttl.sh/sigstore-test:5m --oidc-client-id=\$OIDC_AUTHENTICATION_REALM
            cosign verify --rekor-url=\$REKOR_URL --certificate-identity-regexp ".*@redhat" --certificate-oidc-issuer-regexp ".*keycloak.*" ttl.sh/sigstore-test:5m
      restartPolicy: Never
  backoffLimit: 4 # Defines the number of retries before considering the Job failed.
EOF

# Apply the modified YAML using kubectl
kubectl apply -f job.yaml -n default
oc wait --for=condition=complete job/tas-test-sign-verify --timeout=5m -n default
kubectl logs job/tas-test-sign-verify -n default
