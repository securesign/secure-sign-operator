#!/bin/bash
BASE_DOMAIN=apps.$(oc get dns cluster -o jsonpath='{ .spec.baseDomain }')
OIDC_ISSUER=https://keycloak-keycloak-system.$BASE_DOMAIN/realms/trusted-artifact-signer

cat config/samples/rhtas_v1alpha1_securesign.yaml | sed "s|https://your-oidc-issuer-url|$OIDC_ISSUER|g" > securesign.yaml

oc create ns securesign
oc apply -f securesign.yaml -n securesign

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
        image: registry.redhat.io/rhtas-tech-preview/cosign-rhel9:0.0.2
        env:
        - name: OIDC_AUTHENTICATION_REALM
          value: "trusted-artifact-signer"
        - name: FULCIO_URL
          value: "https://fulcio-server-securesign.${BASE_DOMAIN}"
        - name: OIDC_ISSUER_URL
          value: "https://keycloak-keycloak-system.${BASE_DOMAIN}/realms/trusted-artifact-signer"
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

oc logs -n openshift-rhtas-operator deployment/rhtas-operator-controller-manager

# Apply the modified YAML using kubectl
kubectl apply -f job.yaml -n default
oc wait --for=condition=complete job/tas-test-sign-verify --timeout=5m -n default
