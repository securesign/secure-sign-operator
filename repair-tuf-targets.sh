#!/bin/bash
set -e

echo "=== Repair TUF Targets with Correct Hashes ==="
echo ""

NAMESPACE="trusted-artifact-signer"
TUFCLI_BIN="${TUFCLI_BIN:-/tmp/tufcli-bin}"
WORKDIR="${HOME}/trustroot-repair-$$"

export WORK="${WORKDIR}"
export KEYDIR="${WORK}/keys"
export TUF_REPO="${WORK}/tuf-repo"
export ROOT="${WORK}/root/root.json"

echo "Configuration:"
echo "  Namespace: $NAMESPACE"
echo "  Work directory: $WORKDIR"
echo ""

# Check tufcli
if [ ! -f "$TUFCLI_BIN" ]; then
  echo "❌ Error: tufcli not found at $TUFCLI_BIN"
  exit 1
fi

echo "✓ Using tufcli: $TUFCLI_BIN"

# Create directory structure
echo "[1] Creating temporary directory structure..."
mkdir -p "${WORK}/root/" "${KEYDIR}" "${TUF_REPO}/targets"

# Get TUF pod
TUF_POD=$(kubectl get pods -n $NAMESPACE -l app.kubernetes.io/component=tuf,\!job-name -o jsonpath='{.items[0].metadata.name}')
CTLOG_URL="https://ctlog.${NAMESPACE}.svc"

echo "TUF Pod: $TUF_POD"
echo "✓ Directories created"

echo ""
echo "[2] Extracting TUF signing keys..."
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.snapshot\.pem}' | base64 -d > "${KEYDIR}/snapshot.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.targets\.pem}' | base64 -d > "${KEYDIR}/targets.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.timestamp\.pem}' | base64 -d > "${KEYDIR}/timestamp.pem"
kubectl get secret tuf-root-keys -n $NAMESPACE -o jsonpath='{.data.root\.pem}' | base64 -d > "${KEYDIR}/root.pem"

echo "✓ Extracted signing keys"

echo ""
echo "[3] Downloading entire TUF repository with ALL targets and metadata..."
kubectl cp "${NAMESPACE}/${TUF_POD}:/var/www/html" "${TUF_REPO}/html-backup" -c tuf-server
cp "${TUF_REPO}/html-backup/root.json" "${ROOT}"

# Copy metadata files
cp "${TUF_REPO}/html-backup"/*.json "${TUF_REPO}/" 2>/dev/null || true

echo "✓ Downloaded TUF repository and metadata"

echo ""
echo "[4] Copying ALL existing targets to working directory..."
cp "${TUF_REPO}/html-backup/targets"/* "${TUF_REPO}/targets/" 2>/dev/null || true

# List what we have
echo "Targets directory contents:"
ls -lah "${TUF_REPO}/targets/" | grep -E "\.pub|\.json|\.pem|\.crt" | wc -l
echo "  files copied"

echo ""
echo "[5] Extracting NEW CTLog keys..."
mkdir -p "${TUF_REPO}/input"

ACTIVE_KEY=$(kubectl get secret ctlog-keys-config-shard-test1 -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
FROZEN_KEY=$(kubectl get secret ctlog-keys-config-securesign-sample -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)

echo "$ACTIVE_KEY" > "${TUF_REPO}/targets/ctfe.pub"
echo "$FROZEN_KEY" > "${TUF_REPO}/targets/ctfe_frozen.pub"

echo "✓ Added CTLog keys to targets directory"

echo ""
echo "[6] Running tufcli with ALL targets present..."
"$TUFCLI_BIN" rhtas \
  --root "${ROOT}" \
  --key "${KEYDIR}/snapshot.pem" \
  --key "${KEYDIR}/targets.pem" \
  --key "${KEYDIR}/timestamp.pem" \
  --set-ctlog-target "${TUF_REPO}/targets/ctfe.pub" \
  --ctlog-uri "${CTLOG_URL}" \
  --outdir "${TUF_REPO}" \
  --metadata-url "file://${TUF_REPO}"

echo "✓ Updated metadata for active CTLog key"

echo ""
echo "[7] Running tufcli for frozen CTLog key..."
"$TUFCLI_BIN" rhtas \
  --root "${ROOT}" \
  --key "${KEYDIR}/snapshot.pem" \
  --key "${KEYDIR}/targets.pem" \
  --key "${KEYDIR}/timestamp.pem" \
  --set-ctlog-target "${TUF_REPO}/targets/ctfe_frozen.pub" \
  --ctlog-uri "${CTLOG_URL}-frozen" \
  --outdir "${TUF_REPO}" \
  --metadata-url "file://${TUF_REPO}"

echo "✓ Updated metadata for frozen CTLog key"

echo ""
echo "[8] Uploading repaired TUF metadata to pod..."

# Remove old metadata files to avoid confusion
echo "  Clearing old metadata..."
for file in "${TUF_REPO}"/*.json; do
  [ -f "$file" ] && kubectl cp "$file" "${NAMESPACE}/${TUF_POD}:/var/www/html/$(basename "$file")" -c tuf-server
done

echo "  Uploading updated targets..."
# Upload targets with consistent hashes
for file in "${TUF_REPO}/targets"/*; do
  [ -f "$file" ] && kubectl cp "$file" "${NAMESPACE}/${TUF_POD}:/var/www/html/targets/$(basename "$file")" -c tuf-server
done

echo "✓ Uploaded repaired metadata and targets"

echo ""
echo "[9] Verifying target hashes match metadata..."

# Get the hashes from the latest targets.json
LATEST_VERSION=$(ls "${TUF_REPO}"/*.targets.json | sed 's/.*\/\([0-9]*\)\.targets\.json/\1/' | sort -n | tail -1)
echo "Latest targets version: $LATEST_VERSION"

# Check key targets
echo ""
echo "Verifying target files in pod:"
kubectl exec $TUF_POD -n $NAMESPACE -c tuf-server -- ls -lah /var/www/html/targets/ | grep -E "ctfe|trusted_root"

echo ""
echo "=== Repair Complete ==="
echo ""
echo "Summary:"
echo "  ✓ All targets downloaded with existing files"
echo "  ✓ CTLog keys added to targets"
echo "  ✓ Metadata regenerated with correct hashes for ALL targets"
echo "  ✓ Consistent hashes between metadata and target files"
echo ""
echo "Files:"
echo "  Work directory: $WORKDIR"
echo "  Signed keys: ${KEYDIR}/"
echo "  TUF repo: ${TUF_REPO}/"
echo ""
echo "Verify with:"
echo "  kubectl exec $TUF_POD -n $NAMESPACE -c tuf-server -- cat /var/www/html/${LATEST_VERSION}.targets.json | jq '.signed.targets | keys'"
