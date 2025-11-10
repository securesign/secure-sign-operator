#!/bin/bash
set -e

if [ -n "$IMG" ]; then
  if [[ "$IMG" == *"@"* ]]; then
    IMG_NAME="${IMG%@*}"
    IMG_DIGEST="${IMG#*@}"

    sed -i "s|newName:.*|newName: ${IMG_NAME}|" config/manager/kustomization.yaml
    sed -i "/newTag:/d" config/manager/kustomization.yaml

    if grep -q "digest:" config/manager/kustomization.yaml; then
      sed -i "s|digest:.*|digest: ${IMG_DIGEST}|" config/manager/kustomization.yaml
    else
      sed -i "/newName:/a\  digest: ${IMG_DIGEST}" config/manager/kustomization.yaml
    fi

  elif [[ "$IMG" == *":"* ]]; then
    IMG_NAME="${IMG%%:*}"
    IMG_TAG="${IMG##*:}"

    sed -i "s|newName:.*|newName: ${IMG_NAME}|" config/manager/kustomization.yaml
    sed -i "/digest:/d" config/manager/kustomization.yaml

    if grep -q "newTag:" config/manager/kustomization.yaml; then
      sed -i "s|newTag:.*|newTag: ${IMG_TAG}|" config/manager/kustomization.yaml
    else
      sed -i "/newName:/a\  newTag: ${IMG_TAG}" config/manager/kustomization.yaml
    fi

  else
    sed -i "s|newName:.*|newName: ${IMG}|" config/manager/kustomization.yaml
    sed -i "/digest:/d" config/manager/kustomization.yaml
    sed -i "/newTag:/d" config/manager/kustomization.yaml
  fi

  sed -i "s|^images:|images:\n-|" config/manager/kustomization.yaml
fi

# Build manifests
oc kustomize config/manifests > ./config/manifests/all.yaml

# Generate and validate the Operator bundle
cat ./config/manifests/all.yaml | operator-sdk generate bundle ${BUNDLE_GEN_FLAGS} && operator-sdk bundle validate ./bundle
