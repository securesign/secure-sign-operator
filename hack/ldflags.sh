#!/usr/bin/env bash

# Ensure yq is installed
if ! command -v yq &> /dev/null
then
    echo "yq could not be found, please install it first."
    exit 1
fi

# Path to the YAML file
YAML_FILE="config/default/manager_images_patch.yaml"

# Check if the file exists
if [[ ! -f "$YAML_FILE" ]]; then
    echo "YAML file not found!"
    exit 1
fi

output=$(yq e '.spec.template.spec.containers[] | select(.name == "manager") | .env[] | "-X '\''github.com/securesign/operator/internal/images." + (.name) + "=" + .value + "'\''"' "$YAML_FILE")

# Apply transformations if DEV is true
if [[ "$QUAI_IMAGES" == "true" ]]; then
  output=$(echo "$output" | sed -E -f hack/dev-images.sed)
fi

echo "$output" | tr '\n' ' '

