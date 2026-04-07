#!/bin/bash
set -euo pipefail

# ==============================================================================
# RHTAS: TUF Repository Migration Script (v1.3.2 -> v1.4.0)
# Fixes for logId, validFor.start, and service URLs
# Generates signing_config.v0.2.json if missing
# ==============================================================================

# Ensure correct ENVs are provided safely
if [ -z "${TUF_REPO:-}" ]; then echo "Error: TUF_REPO is not set."; exit 1; fi
if [ -z "${KEYDIR:-}" ]; then echo "Error: KEYDIR is not set."; exit 1; fi
if [ -z "${WORKDIR:-}" ]; then echo "Error: WORKDIR is not set."; exit 1; fi

if [ -z "${FULCIO_URL:-}" ]; then echo "Warning: FULCIO_URL is not set. Skipping FULCIO configuration."; fi
if [ -z "${REKOR_URL:-}" ]; then echo "Warning: REKOR_URL is not set. Skipping REKOR configuration."; fi
if [ -z "${CTLOG_URL:-}" ]; then echo "Warning: CTLOG_URL is not set. Skipping CTLOG configuration."; fi
if [ -z "${TSA_URL:-}" ]; then echo "Warning: TSA_URL is not set. Skipping TSA configuration."; fi
if [ -z "${OIDC_ISSUERS:-}" ]; then echo "Warning: OIDC_ISSUERS is not set. Skipping OIDC configuration."; fi

export METADATA_EXPIRATION="in 52 weeks"
export ROOT="$WORKDIR/root.json"
export NEEDS_UPDATE=false

# --- TAS START TIME - set to the beginning of time to ensure all existing logs are considered valid ---
export TAS_START="1970-01-01T00:00:00Z"

echo "--- Extracting Cosign binary ---"
mkdir -p "$WORKDIR/bin"

if [ -f "$WORKDIR/cosign.gz" ]; then
    echo "Extracting cosign to $WORKDIR/bin/cosign..."
    gunzip -c "$WORKDIR/cosign.gz" > "$WORKDIR/bin/cosign"
    chmod +x "$WORKDIR/bin/cosign"
elif [ ! -f "$WORKDIR/bin/cosign" ] && [ ! -f "$WORKDIR/cosign" ]; then
    echo "Error: cosign binary not found in $WORKDIR."
    exit 1
fi
export PATH="$WORKDIR/bin:$WORKDIR:$PATH"

cosign version >/dev/null || (echo "Error: Cosign not executable or missing."; exit 1)
echo "Cosign binary ready."

# ==============================================================================
# MIGRATION FUNCTIONS
# ==============================================================================

fix_checkpoint_key_id() {
    local target_file="$1"
    echo "Checking for checkpointKeyId..."
    
    if grep -q 'checkpointKeyId' "$target_file"; then
        echo " -> Replacing checkpointKeyId with logId..."
        sed -i 's/"checkpointKeyId"/"logId"/g' "$target_file"
        NEEDS_UPDATE=true
    else
        echo " -> OK: No checkpointKeyId found."
    fi
}

fix_valid_for_start() {
    local target_file="$1"
    echo "Checking for missing validFor.start entries..."
    
    local missing_count
    missing_count=$(python3 -c "
import json, sys
data = json.load(open(sys.argv[1]))
count = 0

# Check tlogs and ctlogs (nested under publicKey)
for key in ['tlogs', 'ctlogs']:
    for log in data.get(key, []):
        if 'start' not in log.get('publicKey', {}).get('validFor', {}):
            count += 1

# Check CAs and TSAs (root level of the item)
for key in ['certificateAuthorities', 'timestampAuthorities']:
    for auth in data.get(key, []):
        if 'start' not in auth.get('validFor', {}):
            count += 1

print(count)
" "$target_file")

    if [ "$missing_count" -gt 0 ]; then
        echo " -> Found $missing_count entries missing validFor.start. Injecting start time ($TAS_START)..."
        # Passing TAS_START as sys.argv[3]
        python3 -c "
import json, sys
data = json.load(open(sys.argv[1]))
start_time = sys.argv[3]

# Update tlogs and ctlogs
for key in ['tlogs', 'ctlogs']:
    for log in data.get(key, []):
        pub_key = log.setdefault('publicKey', {})
        valid_for = pub_key.setdefault('validFor', {})
        if 'start' not in valid_for:
            valid_for['start'] = start_time

# Update CAs and TSAs
for key in ['certificateAuthorities', 'timestampAuthorities']:
    for auth in data.get(key, []):
        valid_for = auth.setdefault('validFor', {})
        if 'start' not in valid_for:
            valid_for['start'] = start_time

with open(sys.argv[2], 'w') as f:
    json.dump(data, f, indent=2)
" "$target_file" "${target_file}.tmp" "$TAS_START"

        mv "${target_file}.tmp" "$target_file"
        echo " -> validFor.start injected successfully."
        NEEDS_UPDATE=true
    else
        echo " -> OK: All entries have validFor.start."
    fi
}

fix_service_urls() {
    local target_file="$1"
    echo "Checking service URLs for internal addresses..."
    
    python3 -c "
import json, sys, os
data = json.load(open(sys.argv[1]))
updated = False

rekor_url = os.environ.get('REKOR_URL')
fulcio_url = os.environ.get('FULCIO_URL')
tsa_url = os.environ.get('TSA_URL')
ctlog_url = os.environ.get('CTLOG_URL')

if rekor_url and 'tlogs' in data:
    for tlog in data['tlogs']:
        if tlog.get('baseUrl') != rekor_url:
            tlog['baseUrl'] = rekor_url
            updated = True

if fulcio_url and 'certificateAuthorities' in data:
    for ca in data['certificateAuthorities']:
        if ca.get('uri') != fulcio_url:
            ca['uri'] = fulcio_url
            updated = True

if tsa_url and 'timestampAuthorities' in data:
    for tsa in data['timestampAuthorities']:
        if tsa.get('uri') != tsa_url:
            tsa['uri'] = tsa_url
            updated = True

if ctlog_url and 'ctlogs' in data:
    for ctlog in data['ctlogs']:
        if ctlog.get('baseUrl') != ctlog_url:
            ctlog['baseUrl'] = ctlog_url
            updated = True

if updated:
    with open(sys.argv[2], 'w') as f:
        json.dump(data, f, indent=2)
    print('true')
else:
    print('false')
" "$target_file" "${target_file}.tmp" > /tmp/url_update_status

    if [ "$(cat /tmp/url_update_status)" = "true" ]; then
        mv "${target_file}.tmp" "$target_file"
        echo " -> Service URLs updated to external routes successfully."
        NEEDS_UPDATE=true
    else
        echo " -> OK: URLs already match external routes."
        rm -f "${target_file}.tmp"
    fi
}

add_signing_config() {
    local target_file="$1"
    local targets_json="$2"
    echo "Checking for signing_config.v0.2.json..."

    local has_config
    has_config=$(python3 -c "import json,sys; print('true' if 'signing_config.v0.2.json' in json.load(open(sys.argv[1]))['signed']['targets'] else 'false')" "$targets_json")

    if [ "$has_config" = "true" ]; then
        echo " -> OK: signing_config.v0.2.json already exists."
        return
    fi

    echo " -> signing_config.v0.2.json is missing. Generating it via Cosign..."

    local cmd=("cosign" "signing-config" "create")

    # Using the new TAS_START constant everywhere
    if [ -n "$FULCIO_URL" ]; then
        cmd+=("--fulcio=url=$FULCIO_URL,api-version=1,start-time=$TAS_START,operator=sigstore.dev")
    fi

    if [ -n "$REKOR_URL" ]; then
        cmd+=("--rekor=url=$REKOR_URL,api-version=1,start-time=$TAS_START,operator=sigstore.dev")
        cmd+=("--rekor-config=ANY")
    fi

    if [ -n "${TSA_URL:-}" ]; then
        cmd+=("--tsa=url=$TSA_URL,api-version=1,start-time=$TAS_START,operator=sigstore.dev")
        cmd+=("--tsa-config=ANY")
    fi

    if [ -n "${OIDC_ISSUERS:-}" ]; then
        IFS=',' read -ra ISSUERS <<< "$OIDC_ISSUERS"
        for issuer in "${ISSUERS[@]}"; do
            cmd+=("--oidc-provider=url=$issuer,api-version=1,start-time=$TAS_START,operator=sigstore.dev")
        done
    fi

    cmd+=("--out" "$WORKDIR/targets/signing_config.v0.2.json")

    "${cmd[@]}"
    
    echo " -> signing_config.v0.2.json generated successfully."
    NEEDS_UPDATE=true
}

# ==============================================================================
# MAIN EXECUTION
# ==============================================================================

mkdir -p "$WORKDIR/targets"

echo "Creating backup file of the TUF repository..."
BACKUP_NAME="$(date +%Y%m%d%H%M%S).backup.tar.gz"
tar -C "$(dirname "$TUF_REPO")" -czf "$WORKDIR/$BACKUP_NAME" --exclude='*.backup.tar.gz' "$(basename "$TUF_REPO")"

cp "$TUF_REPO/root.json" "$ROOT"

# Locate latest trusted_root.json in the TUF repository safely
shopt -s nullglob
TARGET_FILES=("$TUF_REPO"/*.targets.json)
shopt -u nullglob

if [ ${#TARGET_FILES[@]} -eq 0 ]; then
    echo "Error: No .targets.json files found in $TUF_REPO."
    exit 1
fi

TARGETS_FILE=$(printf "%s\n" "${TARGET_FILES[@]}" | sort -t. -k1 -n | tail -1)
TR_HASH=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['signed']['targets']['trusted_root.json']['hashes']['sha256'])" "$TARGETS_FILE")
TR_FILE="$TUF_REPO/targets/${TR_HASH}.trusted_root.json"

if [ ! -f "$TR_FILE" ]; then
    echo "Error: trusted_root.json not found at $TR_FILE."
    exit 1
fi

echo "Targeting trusted_root.json: $TR_FILE"

WORK_TR="$WORKDIR/targets/trusted_root.json"
cp "$TR_FILE" "$WORK_TR"

# --- Execute migration functions  ---
echo "--- Running migration functions ---"
fix_checkpoint_key_id "$WORK_TR"
fix_valid_for_start "$WORK_TR"
fix_service_urls "$WORK_TR"
add_signing_config "$WORK_TR" "$TARGETS_FILE"
echo "-------------------------------"

# --- Finalize ---
if [ "$NEEDS_UPDATE" = true ]; then
    echo "Changes detected. Re-signing trusted_root.json..."

    # Create a safe staging directory
    SAFE_OUTDIR="$WORKDIR/tuf-output"
    mkdir -p "$SAFE_OUTDIR"

    tuftool update \
      --root "$ROOT" \
      --key "$KEYDIR/snapshot.pem" \
      --key "$KEYDIR/targets.pem" \
      --key "$KEYDIR/timestamp.pem" \
      --add-targets "$WORKDIR/targets" \
      --targets-expires "$METADATA_EXPIRATION" \
      --snapshot-expires "$METADATA_EXPIRATION" \
      --timestamp-expires "$METADATA_EXPIRATION" \
      --metadata-url "file://$TUF_REPO" \
      --outdir "$SAFE_OUTDIR"
      
    echo "Migration successful! Syncing back to live volume..."
    # Safely overwrite the live volume with the newly generated files
    cp -a "$SAFE_OUTDIR"/* "$TUF_REPO/"
    
    echo "Re-signing and upload complete."
    
else
    echo "No files changed in the TUF repository. Skipping re-signing."
    rm -f "$WORK_TR"
    
fi

echo "Storing backup file for any catastrophic failure on $TUF_REPO/backup/$BACKUP_NAME"
mkdir -p "$TUF_REPO/backup"
mv "$WORKDIR/$BACKUP_NAME" "$TUF_REPO/backup/$BACKUP_NAME"
