# Configuring Rekor with a KMS Signer Backend

By default, Rekor signs log entries with a key stored in a Kubernetes Secret — the operator generates one automatically if you do not provide it. KMS mode delegates signing to an external key management service, so the private key never resides in the cluster.

Unlike Fulcio and the Timestamp Authority, Rekor signs with a raw key pair rather than a certificate, so **no certificate chain is required**.

## Supported KMS Providers

| Provider | URI Format | Auth Variables |
|----------|-----------|----------------|
| AWS KMS | `awskms:///KEY_ID` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` |
| GCP KMS | `gcpkms://projects/P/locations/L/keyRings/R/cryptoKeys/K/cryptoKeyVersions/V` | `GOOGLE_APPLICATION_CREDENTIALS` |
| Azure Key Vault | `azurekms://VAULT_NAME.vault.azure.net/KEY` | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` |
| HashiCorp Vault | `hashivault://keyname` | `VAULT_ADDR`, `VAULT_TOKEN` |
| OpenBao | `openbao://keyname` | `VAULT_ADDR` or `BAO_ADDR`, `VAULT_TOKEN` or `BAO_TOKEN` |

For Vault/OpenBao, the Transit secrets engine must be enabled. The KMS library checks `VAULT_ADDR` first, then falls back to `BAO_ADDR` (same for token). For non-default mount paths, set `TRANSIT_SECRET_ENGINE_PATH`.

## Example Securesign CR

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Securesign
metadata:
  name: securesign-sample
spec:
  rekor:
    signer:
      type: kms
      kms:
        keyResource: "awskms:///1234abcd-12ab-34cd-56ef-1234567890ab"
    auth:
      env:
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: aws-credentials
              key: access-key-id
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: aws-credentials
              key: secret-access-key
        - name: AWS_REGION
          value: "us-east-1"
  # ... other components (fulcio, ctlog, tuf, etc.)
```

`type` defaults to `secret` — you must explicitly set `type: kms`.

`signer.keyRef` must not be set when `type` is `kms`; the resource is rejected on admission.

For GCP, mount the service account JSON via `spec.rekor.auth.secretMount` and set `GOOGLE_APPLICATION_CREDENTIALS=/var/run/secrets/tas/auth/<key>`, where `<key>` is the `key` field from the `SecretKeySelector`.

## Public Key Discovery

You do not configure Rekor's public key. The operator fetches it from the running service at `/api/v1/log/publicKey` on every reconcile and caches it in `.status.publicKey`, where TUF and other components pick it up.

If the fetched key ever differs from the cached one — for example after rotating the key in the KMS — the operator does **not** accept it automatically, because artifacts signed with the old key would no longer verify. Complete the [Rekor signer key rotation procedure](rekor-key-rotation.md), which includes freezing the current log tree, then acknowledge the new key:

```bash
oc annotate rekor <name> rhtas.redhat.com/refresh-trust-material=true --overwrite -n <namespace>
```

The same applies when switching an existing Rekor instance from `secret` to `kms`.

## Verification

Check that the Rekor resource is `Ready`:
```bash
oc get rekor <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
```

Verify pod args:
```bash
oc get deployment rekor-server -n <namespace> -o jsonpath='{.spec.template.spec.containers[0].args}'
```
Look for `--rekor_server.signer <KMS_KEY_URI>`.

Confirm the served public key matches your KMS key:
```bash
export REKOR_URL=$(oc get rekor <name> -n <namespace> -o jsonpath='{.status.url}')
curl -s "$REKOR_URL/api/v1/log/publicKey"
```

## Related

- [Fulcio KMS Signer](fulcio-kms.md) — KMS signer for the Fulcio CA
- [Timestamp Authority KMS Signer](tsa-kms.md) — KMS signer for the Timestamp Authority
- [Rekor Signer Key Rotation](rekor-key-rotation.md) — rotating the log signing key
- [FIPS](fips.md) — the operator does not validate the signer key in KMS mode; KMS key FIPS compliance is the user's responsibility
