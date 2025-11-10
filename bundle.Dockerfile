ARG VERSION="1.4.0"
ARG CHANNELS="stable,stable-v1.4"
ARG DEFAULT_CHANNEL="stable"
ARG BUNDLE_GEN_FLAGS="-q --overwrite=false --version $VERSION --channels=$CHANNELS --default-channel=$DEFAULT_CHANNEL"
ARG IMG

FROM registry.redhat.io/openshift4/ose-operator-sdk-rhel9@sha256:8ff0cb8587bbca8809490ff59a67496599b6c0cc8e4ca88451481a265f17e581 AS builder

# Copy oc binary from the official image
COPY --from=registry.redhat.io/openshift4/ose-cli-rhel9@sha256:64867e62dbbafe779cdb4233b7c7c8686932717177e5825058e23beccbb3207b /usr/bin/oc /usr/bin/oc

ARG BUNDLE_GEN_FLAGS
ARG IMG

WORKDIR /tmp

COPY ./config/ ./config/
COPY PROJECT .
COPY hack/build-bundle.sh build-bundle.sh

USER root

RUN chmod +x build-bundle.sh
RUN ./build-bundle.sh

FROM scratch

ARG CHANNELS
ARG VERSION

## Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=rhtas-operator
LABEL operators.operatorframework.io.bundle.channels.v1=$CHANNELS
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.38.0-ocp
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4
LABEL operators.openshift.io/valid-subscription="Red Hat Trusted Artifact Signer"

LABEL maintainer="Red Hat, Inc."
LABEL vendor="Red Hat, Inc."
LABEL url="https://www.redhat.com"
LABEL distribution-scope="public"
LABEL version=$VERSION

LABEL description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.description="The bundle image for the rhtas-operator, containing manifests, metadata and testing scorecard."
LABEL io.k8s.display-name="RHTAS operator bundle container image for Red Hat Trusted Artifact Signer."
LABEL io.openshift.tags="rhtas-operator-bundle, rhtas-operator, Red Hat Trusted Artifact Signer."
LABEL summary="Operator Bundle for the rhtas-operator."
LABEL com.redhat.component="rhtas-operator-bundle"
LABEL name="rhtas/rhtas-operator-bundle"
LABEL features.operators.openshift.io/cni="false"
LABEL features.operators.openshift.io/disconnected="true"
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
COPY --from=builder /tmp/bundle/manifests /manifests/
COPY --from=builder /tmp/bundle/metadata /metadata/
COPY --from=builder /tmp/bundle/tests/scorecard /tests/scorecard/
COPY LICENSE /licenses/license.txt
