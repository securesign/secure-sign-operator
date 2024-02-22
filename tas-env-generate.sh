#!/bin/bash

# Initialize variables
BASE_HOSTNAME=""
OIDC_ISSUER_URL=""

# Function to show usage
usage() {
    echo "Usage: $0 -b BASE_HOSTNAME -o OIDC_ISSUER_URL"
    echo "You must provide the endpoint for your OIDC token issuer"
    echo "If connected to an OpenShift cluster, this script can obtain the base hostname for you."
    exit 1
}

# Parse command line options
while getopts "b:o:" opt; do
    case $opt in
        b) BASE_HOSTNAME=$OPTARG ;;
        o) OIDC_ISSUER_URL=$OPTARG ;;
        *) usage ;;
    esac
done

# Check if required options are provided
if [ -z "$OIDC_ISSUER_URL" ]; then
    usage
fi

if [ -z "$BASE_HOSTNAME" ]; then
    # Use either oc or kubectl
    CMD=$(which oc 2>/dev/null)
    if [ -z "$CMD" ]; then
         CMD=$(which kubectl 2>/dev/null)
    fi
    if [ -z "$CMD" ]; then
        echo "No base hostname provided, failed to obtain it from cluster, exiting."
	echo "Use `oc`, `kubectl`, or a cluster administrator to find the base hostname from OpenShift:"
	echo "BASE_HOSTNAME=apps.$($CMD get dns cluster -o jsonpath='{ .spec.baseDomain }')"
	usage
    fi
    BASE_HOSTNAME=apps.$($CMD get dns cluster -o jsonpath='{ .spec.baseDomain }') || true
fi

if [ -z "$BASE_HOSTNAME" ]; then
    echo "No base hostname provided, failed to obtain it from cluster, exiting."
    echo "Use $CMD or ask cluster administrator for base hostname from OpenShift."
    echo "Base hostname can be found with the following command:"
    echo "BASE_HOSTNAME=apps.$($CMD get dns cluster -o jsonpath='{ .spec.baseDomain }')"
	usage
fi

# Export variables
export BASE_HOSTNAME
export OIDC_ISSUER_URL

# Generate the script to initialize the environment variables for the service endpoints
# Write the script to a file
cat <<EOL > tas-env-vars.sh
#!/bin/bash

export BASE_HOSTNAME=apps.$(oc get dns cluster -o jsonpath='{ .spec.baseDomain }')
echo "base hostname = \$BASE_HOSTNAME"

export TUF_URL=https://tuf.\$BASE_HOSTNAME
export COSIGN_FULCIO_URL=https://fulcio.\$BASE_HOSTNAME
export COSIGN_REKOR_URL=https://rekor.\$BASE_HOSTNAME
export COSIGN_MIRROR=\$TUF_URL
export COSIGN_ROOT=\$TUF_URL/root.json
export COSIGN_OIDC_ISSUER=\$OIDC_ISSUER_URL
export COSIGN_CERTIFICATE_OIDC_ISSUER=\$OIDC_ISSUER_URL
export COSIGN_YES="true"

# Gitsign/Sigstore Variables
export SIGSTORE_FULCIO_URL=\$COSIGN_FULCIO_URL
export SIGSTORE_OIDC_ISSUER=\$COSIGN_OIDC_ISSUER
export SIGSTORE_REKOR_URL=\$COSIGN_REKOR_URL

# Rekor CLI Variables
export REKOR_REKOR_SERVER=\$COSIGN_REKOR_URL
EOL

# Make the generated script executable
chmod +x tas-env-vars.sh
echo "A file 'tas-env-vars.sh' to set a local signing environment has been created in the current directory."
echo "To initialize the environment variables, run 'source ./tas-env-vars.sh' from the terminal."