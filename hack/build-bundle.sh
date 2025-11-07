#!/bin/bash
set -e

TOOLS="/tmp"

SRC_TARBALL="/cachi2/output/deps/generic/kustomize-v5.6.0-source.tar.gz"

BUILD_DIR="${TOOLS}/kustomize-src"
KUSTOMIZE="${TOOLS}/kustomize"

mkdir -p "${BUILD_DIR}"

if [ -f "${SRC_TARBALL}" ]; then
  tar -xzf "${SRC_TARBALL}" -C "${BUILD_DIR}" --strip-components=1
else
  curl -L -o "${TOOLS}/kustomize-source.tar.gz" "https://github.com/kubernetes-sigs/kustomize/archive/refs/tags/kustomize/v5.6.0.tar.gz"
  tar -xzf "${TOOLS}/kustomize-source.tar.gz" -C "${BUILD_DIR}" --strip-components=1
fi

cd "${BUILD_DIR}/kustomize"
go build -o "${KUSTOMIZE}"

chmod +x ${KUSTOMIZE}

if [[ -n "$IMG" ]]
then
  pushd config/manager
  ${KUSTOMIZE} edit set image controller="${IMG}"
  popd
fi

${KUSTOMIZE} build config/manifests | operator-sdk generate bundle ${BUNDLE_GEN_FLAGS}

operator-sdk bundle validate ./bundle
