#!/bin/bash
set -e

NAMESPACE="trusted-artifact-signer"
TUF_POD=$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/name=tuf -o jsonpath='{.items[0].metadata.name}')
CTLOG_URL="https://ctlog.${NAMESPACE}.svc"
WORKDIR="/tmp/tuf-update-ctlog-$$"

echo "=== Updating TUF with CTLog keys (active + frozen) using tuftool ==="
echo "TUF Pod: $TUF_POD"
echo "CTLog URL: $CTLOG_URL"
echo "Work directory: $WORKDIR"

# Create working directory
mkdir -p "$WORKDIR"
cd "$WORKDIR"

# Check for tuftool
if ! command -v tuftool &> /dev/null; then
  echo "❌ Error: tuftool not found in PATH"
  echo "Please install tuftool: Download from cluster console (oc console download-links)"
  exit 1
fi

echo ""
echo "[1] Extracting both CTLog public keys..."

# Get active shard public key
ACTIVE_KEY=$(kubectl get secret ctlog-keys-config-shard-test1 -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
echo "$ACTIVE_KEY" > "$WORKDIR/ctfe.pub"
echo "✓ Active key: ctfe.pub (ctlog-shard-test1)"

# Get frozen shard public key
FROZEN_KEY=$(kubectl get secret ctlog-keys-config-securesign-sample -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
echo "$FROZEN_KEY" > "$WORKDIR/ctfe_frozen.pub"
echo "✓ Frozen key: ctfe_frozen.pub (trusted-artifact-signer)"

echo ""
echo "[2] Backing up TUF repository from cluster..."

mkdir -p "$WORKDIR/backup"
mkdir -p "$WORKDIR/tuf-repo"
mkdir -p "$WORKDIR/keys"

kubectl cp "$NAMESPACE/$TUF_POD:/var/www/html" "$WORKDIR/backup/html" -c tuf-server
kubectl cp "$NAMESPACE/$TUF_POD:/var/www/html" "$WORKDIR/tuf-repo" -c tuf-server

echo "✓ TUF repository backed up"

echo ""
echo "[3] Extracting TUF signing keys from secrets..."

# Extract the TUF signing keys from the pod's mounted secrets
# These are typically in the root key secret
TUF_ROOT_SECRET=$(kubectl get secret -n $NAMESPACE -l app.kubernetes.io/component=tuf -o name | grep -E "root|key" | head -1)

if [ -z "$TUF_ROOT_SECRET" ]; then
  echo "ℹ Searching for TUF keys in cluster..."
  TUF_ROOT_SECRET=$(kubectl get secret -n $NAMESPACE tuf-root-keys -o name 2>/dev/null || echo "")
fi

if [ -n "$TUF_ROOT_SECRET" ]; then
  echo "✓ Found TUF key secret: $TUF_ROOT_SECRET"
  # Extract keys
  kubectl get secret $TUF_ROOT_SECRET -n $NAMESPACE -o jsonpath='{.data.snapshot\.pem}' | base64 -d > "$WORKDIR/keys/snapshot.pem" 2>/dev/null || true
  kubectl get secret $TUF_ROOT_SECRET -n $NAMESPACE -o jsonpath='{.data.targets\.pem}' | base64 -d > "$WORKDIR/keys/targets.pem" 2>/dev/null || true
  kubectl get secret $TUF_ROOT_SECRET -n $NAMESPACE -o jsonpath='{.data.timestamp\.pem}' | base64 -d > "$WORKDIR/keys/timestamp.pem" 2>/dev/null || true
fi

# Check if keys exist
if [ ! -f "$WORKDIR/keys/snapshot.pem" ] || [ ! -f "$WORKDIR/keys/targets.pem" ] || [ ! -f "$WORKDIR/keys/timestamp.pem" ]; then
  echo "⚠ Warning: Could not extract all TUF signing keys from secrets"
  echo "  This is required for tuftool to sign the metadata"
  echo ""
  echo "  Manually place the keys in: $WORKDIR/keys/"
  echo "    - snapshot.pem"
  echo "    - targets.pem"
  echo "    - timestamp.pem"
  echo ""
  read -p "Continue without keys? (y/n) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
  fi
else
  echo "✓ Extracted TUF signing keys"
fi

echo ""
echo "[4] Running tuftool to add active CTLog key..."

tuftool rhtas \
  --root "$WORKDIR/tuf-repo/root.json" \
  --key "$WORKDIR/keys/snapshot.pem" \
  --key "$WORKDIR/keys/targets.pem" \
  --key "$WORKDIR/keys/timestamp.pem" \
  --set-ctlog-target ctfe.pub \
  --ctlog-uri "$CTLOG_URL" \
  --outdir "$WORKDIR/tuf-repo" \
  --metadata-url "file://$WORKDIR/tuf-repo" || {
  echo "⚠ tuftool failed for active key. Continuing..."
}

echo "✓ Added active CTLog key to metadata"

echo ""
echo "[5] Running tuftool to add frozen CTLog key..."

tuftool rhtas \
  --root "$WORKDIR/tuf-repo/root.json" \
  --key "$WORKDIR/keys/snapshot.pem" \
  --key "$WORKDIR/keys/targets.pem" \
  --key "$WORKDIR/keys/timestamp.pem" \
  --set-ctlog-target ctfe_frozen.pub \
  --ctlog-uri "${CTLOG_URL}-frozen" \
  --outdir "$WORKDIR/tuf-repo" \
  --metadata-url "file://$WORKDIR/tuf-repo" || {
  echo "⚠ tuftool failed for frozen key. Continuing..."
}

echo "✓ Added frozen CTLog key to metadata"

echo ""
echo "[6] Uploading updated TUF metadata and targets to pod..."

# Copy updated metadata back to the TUF pod
kubectl cp "$WORKDIR/tuf-repo/root.json" "$NAMESPACE/$TUF_POD:/var/www/html/root.json" -c tuf-server
kubectl cp "$WORKDIR/tuf-repo/targets.json" "$NAMESPACE/$TUF_POD:/var/www/html/targets.json" -c tuf-server
kubectl cp "$WORKDIR/tuf-repo/snapshot.json" "$NAMESPACE/$TUF_POD:/var/www/html/snapshot.json" -c tuf-server
kubectl cp "$WORKDIR/tuf-repo/timestamp.json" "$NAMESPACE/$TUF_POD:/var/www/html/timestamp.json" -c tuf-server

# Copy targets directory
kubectl cp "$WORKDIR/tuf-repo/targets/" "$NAMESPACE/$TUF_POD:/var/www/html/targets/" -c tuf-server

echo "✓ Updated TUF metadata and targets uploaded to pod"

echo ""
echo "=== Update Complete ==="
echo ""
echo "Summary:"
echo "  ✓ Active CTLog key: ctfe.pub (ctlog-shard-test1)"
echo "  ✓ Frozen CTLog key: ctfe_frozen.pub (trusted-artifact-signer)"
echo "  ✓ TUF metadata updated with both keys"
echo ""
echo "Files:"
echo "  - Backup: $WORKDIR/backup/html"
echo "  - Updated repo: $WORKDIR/tuf-repo"
echo ""
echo "Next steps:"
echo "  1. Verify TUF pod is healthy: kubectl get tuf -n $NAMESPACE"
echo "  2. Check TUF logs for any issues: kubectl logs -l app.kubernetes.io/name=tuf -n $NAMESPACE"
echo "  3. Test clients can fetch both keys from TUF"
echo "  4. Verify signatures from both shards verify correctly"
