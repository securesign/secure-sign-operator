#!/bin/bash
set -e

NAMESPACE="trusted-artifact-signer"
TUF_POD=$(kubectl get pod -n $NAMESPACE -l app.kubernetes.io/name=tuf -o jsonpath='{.items[0].metadata.name}')
WORKDIR="/tmp/tuf-update-$$"

echo "=== Updating TUF with separate CTLog keys ==="
echo "TUF Pod: $TUF_POD"
echo "Namespace: $NAMESPACE"
echo "Work directory: $WORKDIR"

# Create working directory
mkdir -p "$WORKDIR"
cd "$WORKDIR"

echo ""
echo "[1] Extracting both CTLog public keys from secrets..."

# Get active shard public key (ctlog-shard-test1)
ACTIVE_KEY=$(kubectl get secret ctlog-keys-config-shard-test1 -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
echo "✓ Active shard public key (ctlog-shard-test1) extracted"

# Get frozen shard public key (trusted-artifact-signer)
FROZEN_KEY=$(kubectl get secret ctlog-keys-config-securesign-sample -n $NAMESPACE -o jsonpath='{.data.public}' | base64 -d)
echo "✓ Frozen shard public key (trusted-artifact-signer) extracted"

echo ""
echo "[2] Backing up TUF repository..."

# Backup TUF repo
mkdir -p "$WORKDIR/backup"
kubectl cp "$NAMESPACE/$TUF_POD:/var/www/html" "$WORKDIR/backup/html" -c tuf-server
echo "✓ TUF repository backed up to $WORKDIR/backup/html"

echo ""
echo "[3] Creating separate key files..."

# Create active key file
echo "$ACTIVE_KEY" > "$WORKDIR/ctfe.pub"
echo "✓ ctfe.pub created (active shard: ctlog-shard-test1)"

# Create frozen key file
echo "$FROZEN_KEY" > "$WORKDIR/ctfe_frozen.pub"
echo "✓ ctfe_frozen.pub created (frozen shard: trusted-artifact-signer)"

echo ""
echo "[4] Updating TUF metadata..."

# Copy active key to TUF pod as ctfe.pub (for backward compatibility and active shard)
kubectl cp "$WORKDIR/ctfe.pub" "$NAMESPACE/$TUF_POD:/var/www/html/ctfe.pub" -c tuf-server
echo "✓ Updated ctfe.pub with active shard key"

# Copy frozen key to TUF pod as ctfe_frozen.pub
kubectl cp "$WORKDIR/ctfe_frozen.pub" "$NAMESPACE/$TUF_POD:/var/www/html/ctfe_frozen.pub" -c tuf-server
echo "✓ Updated ctfe_frozen.pub with frozen shard key"

echo ""
echo "=== Update Complete ==="
echo ""
echo "Summary:"
echo "  - ctfe.pub: Active shard (ctlog-shard-test1)"
echo "  - ctfe_frozen.pub: Frozen shard (trusted-artifact-signer)"
echo ""
echo "Files saved in TUF:"
echo "  - /var/www/html/ctfe.pub (active)"
echo "  - /var/www/html/ctfe_frozen.pub (frozen)"
echo ""
echo "Local files:"
echo "  - $WORKDIR/ctfe.pub (active)"
echo "  - $WORKDIR/ctfe_frozen.pub (frozen)"
echo "  - $WORKDIR/backup/html (TUF backup)"
echo ""
echo "Next steps:"
echo "  1. Verify clients can fetch both keys from TUF"
echo "  2. Update client configurations to use ctfe_frozen.pub for old signatures"
echo "  3. Keep the backup in a safe location"
