# Configuring Fulcio with a KMS Signer Backend

By default, Fulcio uses a file-based signer where the CA private key is stored in a Kubernetes Secret. KMS mode delegates private key operations to an external key management service.

## Supported KMS Providers

| Provider | URI Format | Auth Variables |
|----------|-----------|----------------|
| AWS KMS | `awskms:///KEY_ID` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` |
| GCP KMS | `gcpkms://projects/P/locations/L/keyRings/R/cryptoKeys/K/cryptoKeyVersions/V` | `GOOGLE_APPLICATION_CREDENTIALS` |
| Azure Key Vault | `azurekms://VAULT_NAME.vault.azure.net/KEY` | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` |
| HashiCorp Vault | `hashivault://keyname` | `VAULT_ADDR`, `VAULT_TOKEN` |
| OpenBao | `openbao://keyname` | `VAULT_ADDR` or `BAO_ADDR`, `VAULT_TOKEN` or `BAO_TOKEN` |

For Vault/OpenBao, the Transit secrets engine must be enabled. The KMS library checks `VAULT_ADDR` first, then falls back to `BAO_ADDR` (same for token). For non-default mount paths, set `TRANSIT_SECRET_ENGINE_PATH`.

## Preparing the CA Certificate Chain

KMS holds only the private key. You must create a CA certificate chain externally and provide it as a Secret.

> **Important:** The certificate chain's public key **must** match the KMS signing key. Fulcio verifies this at startup and will fail with a fatal error if the keys differ. For Vault/OpenBao, do **not** mark the transit key `exportable` — certificate generation only needs the public key and remote signing via the Transit `sign` endpoint, so the private key never has to leave the vault.

The PEM file must be ordered **signing CA first, root CA last**: issuing (intermediate) CA certificate, then root CA. For a single self-signed root CA (common in dev/test), the chain contains just one certificate. See [upstream CA certificate requirements](https://github.com/sigstore/fulcio/blob/main/docs/setup.md) for key usage and constraints.

Fulcio provides a [`certificate-maker`](https://github.com/sigstore/fulcio/blob/main/docs/certificate-maker.md) tool for generating compliant chains from existing KMS keys.

Create the secret:
```bash
oc create secret generic fulcio-kms-cert --from-file=cert=chain.pem -n <namespace>
```

## Example Securesign CR

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Securesign
metadata:
  name: securesign-sample
spec:
  fulcio:
    signer:
      type: kms
      kms:
        keyResource: "awskms:///1234abcd-12ab-34cd-56ef-1234567890ab"
      certificateChain:
        certificateChainRef:
          name: fulcio-kms-cert
          key: cert
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
    config:
      oidcIssuers:
        - issuer: "https://your-oidc-issuer"
          clientID: "trusted-artifact-signer"
          type: email
    ingress:
      enabled: true
  # ... other components (rekor, ctlog, tuf, etc.)
```

`type` defaults to `file` — you must explicitly set `type: kms`.

Both `kms.keyResource` and `certificateChain.certificateChainRef` are required for KMS mode.

For GCP, mount the service account JSON via `spec.fulcio.auth.secretMount` and set `GOOGLE_APPLICATION_CREDENTIALS=/var/run/secrets/tas/auth/<key>`, where `<key>` is the `key` field from the `SecretKeySelector`.

## Verification

Check that `FulcioCertAvailable` condition is `True`:
```bash
oc get fulcio <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="FulcioCertAvailable")].status}'
```

Verify pod args:
```bash
oc get deployment fulcio-server -n <namespace> -o jsonpath='{.spec.template.spec.containers[0].args}'
```
Look for `--ca=kmsca`, `--kms-resource`, and `--kms-cert-chain-path`.

Test signing. Initialize TUF against your Securesign instance, then sign — cosign discovers the Fulcio/Rekor/OIDC endpoints from the TUF-provided signing config, so no `--fulcio-url`/`--rekor-url` flags are needed:
```bash
export TUF_URL=$(oc get tuf -o jsonpath='{.items[0].status.url}' -n <namespace>)
cosign initialize --mirror "$TUF_URL" --root "$TUF_URL/root.json"
cosign sign --oidc-client-id trusted-artifact-signer <image>
```

## Related

- [Fulcio Certificate Rotation](fulcio-key-rotation.md) — rotating KMS keys and certificate chains
- [FIPS](fips.md) — KMS key FIPS compliance is the user's responsibility
