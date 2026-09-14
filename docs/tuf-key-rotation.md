# Updating the TUF Repository During Key Rotation

When a Fulcio certificate, Rekor signer key, CTLog key, or Timestamp Authority
certificate chain is rotated, update the TUF repository so clients can verify
artifacts created with the old trust material and use the new trust material for
future artifacts.

This procedure uses [`tufcli`](https://github.com/securesign/tufcli)'s `rhtas`
command. It marks the old target as `Expired`, adds the new target as `Active`,
and regenerates and signs `targets.json`, `snapshot.json`, and `timestamp.json`.

## Prerequisites

Before you begin, ensure that:

1. You have access to the Kubernetes cluster and permission to read and update the TUF service pod.
2. `tufcli` is installed and available in `PATH`.
3. You have the new trust material in a PEM file: the Fulcio certificate chain, Rekor public key, CTLog public key, or TSA certificate chain.
4. You have the TUF signing keys for the `snapshot`, `targets`, and `timestamp` roles. Keep these keys secure and do not commit them to the repository.

The old target must be retained in the TUF repository and have a distinct
filename from the new target. This allows clients verifying older artifacts to
continue using the old trust material.

## Prepare a local working copy

Set the namespace, TUF pod name, and working directory. The TUF pod name can be
found with:

```bash
export NAMESPACE=<namespace>
export TUF_POD=$(kubectl get pods -n "$NAMESPACE" \
  -l app.kubernetes.io/name=tuf -o jsonpath='{.items[0].metadata.name}')
export WORKDIR=$(mktemp -d)
mkdir -p "$WORKDIR/keys" "$WORKDIR/tuf-repo"
```

Copy the currently served repository from the TUF pod:

```bash
kubectl cp "$NAMESPACE/$TUF_POD:/var/www/html/." "$WORKDIR/tuf-repo" \
  -c tuf
```

Copy the TUF role keys from the Kubernetes Secret. Adapt the loop if your
deployment uses different Secret keys:

```bash
for key in snapshot.pem targets.pem timestamp.pem; do
  kubectl get secret tuf-root-keys -n "$NAMESPACE" \
    -o jsonpath="{.data.$key}" | base64 -d > "$WORKDIR/keys/$key"
  chmod 600 "$WORKDIR/keys/$key"
done
```

Check that the working copy contains the repository root metadata and targets:

```bash
ls "$WORKDIR/tuf-repo/root.json" "$WORKDIR/tuf-repo/targets"
```

## Mark the old target as expired

Copy the currently trusted target to the working directory, or use the copy
from the repository's `targets` directory. The target filename is the basename
that appears in `targets.json`.

For example, for a TSA rotation:

```bash
cp "$WORKDIR/tuf-repo/targets/tsa.certchain.pem" "$WORKDIR/tsa.certchain.pem"

tufcli rhtas \
  --root "$WORKDIR/tuf-repo/root.json" \
  --key "$WORKDIR/keys/snapshot.pem" \
  --key "$WORKDIR/keys/targets.pem" \
  --key "$WORKDIR/keys/timestamp.pem" \
  --set-tsa-target "$WORKDIR/tsa.certchain.pem" \
  --tsa-uri <tsa-url> \
  --tsa-status Expired \
  --outdir "$WORKDIR/tuf-repo" \
  --metadata-url "file://$WORKDIR/tuf-repo"
```

Use the corresponding target and URI flags for another service:

| Service | Target flag | URI flag |
| --- | --- | --- |
| Fulcio | `--set-fulcio-target` | `--fulcio-uri` |
| CTLog | `--set-ctlog-target` | `--ctlog-uri` |
| Rekor | `--set-rekor-target` | `--rekor-uri` |
| TSA | `--set-tsa-target` | `--tsa-uri` |

## Add the new target

Save the new certificate, certificate chain, or public key as a different
filename. Then add it as an `Active` target using the same service URI:

```bash
export NEW_TARGET=<path-to-new-target.pem>

tufcli rhtas \
  --root "$WORKDIR/tuf-repo/root.json" \
  --key "$WORKDIR/keys/snapshot.pem" \
  --key "$WORKDIR/keys/targets.pem" \
  --key "$WORKDIR/keys/timestamp.pem" \
  --set-tsa-target "$NEW_TARGET" \
  --tsa-uri <tsa-url> \
  --tsa-status Active \
  --outdir "$WORKDIR/tuf-repo" \
  --metadata-url "file://$WORKDIR/tuf-repo"
```

Replace the TSA flags when rotating Fulcio, CTLog, or Rekor. `tufcli` uses the
basename of `NEW_TARGET` as the TUF target name, so do not reuse the old target
filename.

Review the generated repository before publishing it. Confirm that both the
old `Expired` target and the new `Active` target are present in `targets.json`,
and that metadata versions and signatures were updated.

## Publish the updated repository

Copy the complete repository, including `metadata` and `targets`, back to the
TUF pod:

```bash
kubectl cp "$WORKDIR/tuf-repo/." "$NAMESPACE/$TUF_POD:/var/www/html" \
  -c tuf
```

Verify that the new target is served and that the TUF pod remains ready:

```bash
kubectl exec -n "$NAMESPACE" "$TUF_POD" -c tuf -- \
  test -f "/var/www/html/targets/$(basename "$NEW_TARGET")"
kubectl get pod "$TUF_POD" -n "$NAMESPACE"
```

After publishing the TUF repository, complete the component-specific rotation
procedure and acknowledge the new trust material on the rotated resource, for
example:

```bash
kubectl annotate timestampauthority <name> \
  rhtas.redhat.com/refresh-trust-material=true --overwrite \
  -n "$NAMESPACE"
```

See [Timestamp Authority key rotation](tsa-key-rotation.md), [Fulcio
certificate rotation](fulcio-key-rotation.md), [Rekor signer key
rotation](rekor-key-rotation.md), and [CTLog key rotation](ctlog-key-rotation.md)
for the service-specific steps.
