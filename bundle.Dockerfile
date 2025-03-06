FROM registry.access.redhat.com/ubi9/ubi-micro@sha256:63949c93d85abc5385985722c8fe9518782ff280828fc9b283419e4c0fdf05c2

## Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=rhtas-operator
LABEL operators.operatorframework.io.bundle.channels.v1=stable,stable-v1.2
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.37.0
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v3
LABEL operators.openshift.io/valid-subscription="Red Hat Trusted Artifact Signer"

LABEL description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.display-name="RHTAS operator bundle container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="rhtas-operator-bundle, rhtas-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator Bundle for the rhtas-operator."
LABEL com.redhat.component="sigstore-operator-bundle"
LABEL name="sigstore-operator-bundle"
LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/disconnected="false"
LABEL features.operators.openshift.io/fips-compliant="false"
LABEL features.operators.openshift.io/proxy-aware="false"
LABEL features.operators.openshift.io/cnf="false"
LABEL features.operators.openshift.io/csi="false"
LABEL features.operators.openshift.io/tls-profiles="false"
LABEL features.operators.openshift.io/token-auth-aws="false"
LABEL features.operators.openshift.io/token-auth-azure="false"
LABEL features.operators.openshift.io/token-auth-gcp="false"

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/
