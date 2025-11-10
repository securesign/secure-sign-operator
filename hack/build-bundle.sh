#!/bin/bash
set -e

KUSTOMIZATION_FILE="config/manager/kustomization.yaml"

if [ -n "$IMG" ]; then
  if [[ "$IMG" == *"@"* ]]; then
    IMG_NAME="${IMG%@*}"
    IMG_DIGEST="${IMG#*@}"

    sed -i "s|newName:.*|newName: ${IMG_NAME}|" "${KUSTOMIZATION_FILE}"
    sed -i "/newTag:/d" "${KUSTOMIZATION_FILE}"

    if grep -q "digest:" "${KUSTOMIZATION_FILE}"; then
      sed -i "s|digest:.*|digest: ${IMG_DIGEST}|" "${KUSTOMIZATION_FILE}"
    else
      sed -i "/newName:/a\  digest: ${IMG_DIGEST}" "${KUSTOMIZATION_FILE}"
    fi

  elif [[ "$IMG" == *":"* ]]; then
    IMG_NAME="${IMG%%:*}"
    IMG_TAG="${IMG##*:}"

    sed -i "s|newName:.*|newName: ${IMG_NAME}|" "${KUSTOMIZATION_FILE}"
    sed -i "/digest:/d" "${KUSTOMIZATION_FILE}"

    if grep -q "newTag:" "${KUSTOMIZATION_FILE}"; then
      sed -i "s|newTag:.*|newTag: ${IMG_TAG}|" "${KUSTOMIZATION_FILE}"
    else
      sed -i "/newName:/a\  newTag: ${IMG_TAG}" "${KUSTOMIZATION_FILE}"
    fi

  else
    sed -i "s|newName:.*|newName: ${IMG}|" "${KUSTOMIZATION_FILE}"
    sed -i "/digest:/d" "${KUSTOMIZATION_FILE}"
    sed -i "/newTag:/d" "${KUSTOMIZATION_FILE}"
  fi

  sed -i "s|^images:|images:\n-|" "${KUSTOMIZATION_FILE}"
fi

# Build manifests
oc kustomize config/manifests > ./config/manifests/all.yaml

# Generate and validate the Operator bundle
cat ./config/manifests/all.yaml | operator-sdk generate bundle ${BUNDLE_GEN_FLAGS} && operator-sdk bundle validate ./bundle
