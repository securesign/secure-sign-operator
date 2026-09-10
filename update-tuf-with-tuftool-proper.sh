#!/bin/bash
set -e

echo "=== Update TUF with CTLog Keys (Active + Frozen) using tuftool ==="
echo ""

# Use extracted tufcli or tuftool if available
TUFCLI_BIN="${TUFCLI_BIN:-/tmp/tufcli-bin}"
if [ ! -f "$TUFCLI_BIN" ]; then
  if command -v tuftool &> /dev/null; then
    TUFCLI_BIN="tuftool"
  else
    echo "❌ Error: tufcli not found at $TUFCLI_BIN"
    echo "Please extract it first:"
    echo "  podman run --rm quay.io/securesign/tufcli@sha256:f9057c3d88d08fba2b46fa18b526f6a994e017fd1582147d55d0df4eb986740d..."
    exit 1
  fi
fi

echo "Using tufcli: $TUFCLI_BIN"
"$TUFCLI_BIN" --version

# Configuration
NAMESPACE="trusted-artifact-signer"
WORKDIR="${HOME}/trustroot-ctlog-update-$$"
export WORK="${WORKDIR}"
export KEYDIR="${WORK}/keys"
export INPUT="${WORK}/input"
export TUF_REPO="${WORK}/tuf-repo"
export ROOT="${WORK}/root/root.json"

echo "Configuration:"
echo "  Namespace: $NAMESPACE"
echo "  Work directory: $WORKDIR"
echo ""

# Get TUF pod and URLs
export TUF_SERVER_POD="$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/component=tuf,\!job-name -o jsonpath='{.items[0].metadata.name}')"
export CTLOG_URL="https://ctlog.${NAMESPACE}.svc"

echo "Cluster resources:"
echo "  TUF Pod: $TUF_SERVER_POD"
echo "  CTLog URL: $CTLOG_URL"
echo ""

# Create directory structure
echo "[1] Creating temporary directory structure..."
mkdir -p "${WORK}/root/" "${KEYDIR}" "${INPUT}" "${TUF_REPO}"
echo "✓ Created directories"

echo ""
echo "[2] Extracting TUF signing keys from secret/tuf-root-keys..."
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.snapshot\.pem}' | base64 -d > "${KEYDIR}/snapshot.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.targets\.pem}' | base64 -d > "${KEYDIR}/targets.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.timestamp\.pem}' | base64 -d > "${KEYDIR}/timestamp.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.root\.pem}' | base64 -d > "${KEYDIR}/root.pem"

if [ ! -f "${KEYDIR}/timestamp.pem" ] || [ ! -f "${KEYDIR}/snapshot.pem" ] || [ ! -f "${KEYDIR}/targets.pem" ]; then
  echo "❌ Error: Failed to extract signing keys"
  echo "Available keys:"
  ls -la "${KEYDIR}/"
  exit 1
fi

echo "✓ Extracted signing keys:"
ls -lah "${KEYDIR}/"*.pem

echo ""
echo "[3] Downloading TUF repository from pod..."
kubectl cp "${NAMESPACE}/${TUF_SERVER_POD}:/var/www/html" "${TUF_REPO}" -c tuf-server
cp "${TUF_REPO}/root.json" "${ROOT}"
echo "✓ Downloaded TUF repository"
echo "  Root.json: $ROOT"

echo ""
echo "[4] Extracting CTLog public keys..."
ACTIVE_KEY=$(kubectl get secret ctlog-keys-config-shard-test1 -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
FROZEN_KEY=$(kubectl get secret ctlog-keys-config-securesign-sample -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)

echo "$ACTIVE_KEY" > "${INPUT}/ctfe.pub"
echo "$FROZEN_KEY" > "${INPUT}/ctfe_frozen.pub"

echo "✓ Extracted CTLog keys:"
echo "  Active key: ${INPUT}/ctfe.pub"
echo "  Frozen key: ${INPUT}/ctfe_frozen.pub"

echo ""
echo "[5] Using tuftool to add active CTLog key..."
"$TUFCLI_BIN" rhtas \
  --root "${ROOT}" \
  --key "${KEYDIR}/snapshot.pem" \
  --key "${KEYDIR}/targets.pem" \
  --key "${KEYDIR}/timestamp.pem" \
  --set-ctlog-target "${INPUT}/ctfe.pub" \
  --ctlog-uri "${CTLOG_URL}" \
  --outdir "${TUF_REPO}" \
  --metadata-url "file://${TUF_REPO}"

if [ $? -eq 0 ]; then
  echo "✓ Added active CTLog key (ctlog-shard-test1)"
else
  echo "⚠ tuftool command for active key had issues"
fi

echo ""
echo "[6] Using tuftool to add frozen CTLog key..."
"$TUFCLI_BIN" rhtas \
  --root "${ROOT}" \
  --key "${KEYDIR}/snapshot.pem" \
  --key "${KEYDIR}/targets.pem" \
  --key "${KEYDIR}/timestamp.pem" \
  --set-ctlog-target "${INPUT}/ctfe_frozen.pub" \
  --ctlog-uri "${CTLOG_URL}-frozen" \
  --outdir "${TUF_REPO}" \
  --metadata-url "file://${TUF_REPO}"

if [ $? -eq 0 ]; then
  echo "✓ Added frozen CTLog key (trusted-artifact-signer)"
else
  echo "⚠ tuftool command for frozen key had issues"
fi

echo ""
echo "[7] Verifying metadata files..."
echo "Generated metadata files:"
ls -lah "${TUF_REPO}"/*.json

echo ""
echo "[8] Uploading updated TUF repository to pod..."
# Upload all metadata files
for file in "${TUF_REPO}"/*.json; do
  [ -f "$file" ] && kubectl cp "$file" "${NAMESPACE}/${TUF_SERVER_POD}:/var/www/html/$(basename "$file")" -c tuf-server
done

echo "✓ Uploaded updated TUF metadata"

echo ""
echo "[9] Verifying updates in pod..."
echo "Targets directory in pod:"
kubectl exec $TUF_SERVER_POD -n $NAMESPACE -c tuf-server -- ls -lah /var/www/html/targets/ | grep ctfe

echo ""
echo "=== Update Complete ==="
echo ""
echo "Summary:"
echo "  ✓ Active CTLog key added to TUF (ctlog-shard-test1)"
echo "  ✓ Frozen CTLog key added to TUF (trusted-artifact-signer)"
echo "  ✓ TUF metadata updated with tuftool"
echo "  ✓ Metadata signed with TUF signing keys"
echo "  ✓ Changes uploaded to TUF pod"
echo ""
echo "Next steps:"
echo "  1. Verify TUF pod is healthy:"
echo "     kubectl get tuf securesign-sample -n $NAMESPACE"
echo ""
echo "  2. Check TUF pod logs:"
echo "     kubectl logs -l app.kubernetes.io/name=tuf -n $NAMESPACE --tail=50"
echo ""
echo "  3. Test client can fetch the keys:"
echo "     curl -s https://ctlog.${NAMESPACE}.svc/ctfe.pub"
echo "     curl -s https://ctlog.${NAMESPACE}.svc/ctfe_frozen.pub"
echo ""
echo "  4. Verify clients can verify signatures from both shards"
echo ""
echo "Files location: $WORKDIR"
echo "  - $KEYDIR/ (signing keys)"
echo "  - $TUF_REPO/ (updated TUF repository)"
echo ""
echo "⚠ Keep these files safe! They contain TUF signing keys."
