#!/bin/bash
set -e

TOOLS="/tmp"

if [ -f "/cachi2/output/deps/generic/kustomize_v5.6.0_linux_amd64.tar.gz" ]
then
  tar -xzf /cachi2/output/deps/generic/kustomize_v5.6.0_linux_amd64.tar.gz -C ${TOOLS}
  KUSTOMIZE=${TOOLS}/kustomize
else
  curl -Lo ${TOOLS}/kustomize.tar.gz "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.6.0/kustomize_v5.6.0_linux_amd64.tar.gz" && \
  tar -xzf ${TOOLS}/kustomize.tar.gz -C ${TOOLS}
  rm ${TOOLS}/kustomize.tar.gz
  KUSTOMIZE=${TOOLS}/kustomize
fi
chmod +x ${KUSTOMIZE}

if [[ -n "$IMG" ]]
then
  pushd config/manager
  ${KUSTOMIZE} edit set image controller="${IMG}"
  popd
fi

${KUSTOMIZE} build config/manifests | operator-sdk generate bundle ${BUNDLE_GEN_FLAGS}

operator-sdk bundle validate ./bundle
