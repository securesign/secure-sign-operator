# Configuring the Timestamp Authority with a KMS Signer Backend

By default, the Timestamp Authority (TSA) uses a file-based signer where the private key is stored in a Kubernetes Secret. KMS mode delegates private key operations to an external key management service.

The TSA also supports a Tink signer, which is configured separately and uses a different URI format (`gcp-kms://`, `aws-kms://`, `hcvault://`). This document covers the `kms` signer only.

## Supported KMS Providers

| Provider | URI Format | Auth Variables |
|----------|-----------|----------------|
| AWS KMS | `awskms:///KEY_ID` | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` |
| GCP KMS | `gcpkms://projects/P/locations/L/keyRings/R/cryptoKeys/K/cryptoKeyVersions/V` | `GOOGLE_APPLICATION_CREDENTIALS` |
| Azure Key Vault | `azurekms://VAULT_NAME.vault.azure.net/KEY` | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` |
| HashiCorp Vault | `hashivault://keyname` | `VAULT_ADDR`, `VAULT_TOKEN` |
| OpenBao | `openbao://keyname` | `VAULT_ADDR` or `BAO_ADDR`, `VAULT_TOKEN` or `BAO_TOKEN` |

For Vault/OpenBao, the Transit secrets engine must be enabled. The KMS library checks `VAULT_ADDR` first, then falls back to `BAO_ADDR` (same for token). For non-default mount paths, set `TRANSIT_SECRET_ENGINE_PATH`.

## Preparing the Certificate Chain

KMS holds only the private key. You must create a certificate chain externally and provide it as a Secret.

> **Important:** The leaf certificate's public key **must** match the KMS signing key. For Vault/OpenBao, do **not** mark the transit key `exportable` — issuing the leaf certificate only needs the public key and remote signing via the Transit `sign` endpoint, so the private key never has to leave the vault.

Requirements for the chain:

- Ordered **leaf first, root last**: leaf (timestamping) certificate, then any intermediate CAs, then the root CA.
- The leaf certificate must carry the `timeStamping` extended key usage marked **critical**, and `digitalSignature` key usage.
- The leaf's public key is the KMS key's public key; the leaf is signed by your intermediate or root CA.

See the upstream [Timestamp Authority cloud KMS documentation](https://github.com/securesign/timestamp-authority?tab=readme-ov-file#cloud-kms) for details, and the `fetch-tsa-certs` binary from the command line tools for retrieving a chain from a KMS-backed deployment.

Create the secret:
```bash
oc create secret generic tsa-kms-cert --from-file=cert=chain.pem -n <namespace>
```

## Example Securesign CR

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Securesign
metadata:
  name: securesign-sample
spec:
  tsa:
    signer:
      type: kms
      kms:
        keyResource: "awskms:///1234abcd-12ab-34cd-56ef-1234567890ab"
      certificateChain:
        certificateChainRef:
          name: tsa-kms-cert
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
    ingress:
      enabled: true
  # ... other components (fulcio, rekor, ctlog, tuf, etc.)
```

`type` defaults to `file` — you must explicitly set `type: kms`.

Both `kms.keyResource` and `certificateChain.certificateChainRef` are required for KMS mode. The `certificateChain.rootCA`, `intermediateCA`, and `leafCA` fields are for operator-generated chains only and must not be set alongside `certificateChainRef`.

For GCP, mount the service account JSON via `spec.tsa.auth.secretMount` and set `GOOGLE_APPLICATION_CREDENTIALS=/var/run/secrets/tas/auth/<key>`, where `<key>` is the `key` field from the `SecretKeySelector`.

## Verification

Check that the `TSASignerCondition` condition is `True`:
```bash
oc get timestampauthority <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="TSASignerCondition")].status}'
```

Verify the pod command. The TSA passes its flags via `command`, not `args`:
```bash
oc get deployment tsa-server -n <namespace> -o jsonpath='{.spec.template.spec.containers[0].command}'
```
Look for `--timestamp-signer=kms` and `--kms-key-resource=<KMS_KEY_URI>`.

Inspect the certificate chain the running service is serving, which the operator resolves into the status for TUF and other components:
```bash
oc get timestampauthority <name> -n <namespace> -o jsonpath='{.status.certificateChain}'
```

## Rotating the KMS Key

If the certificate chain served by the running service ever differs from the cached one — for example after rotating the key in the KMS — the operator does **not** accept it automatically, because artifacts timestamped with the old key would no longer verify. Complete the [Timestamp Authority key rotation procedure](tsa-key-rotation.md), then acknowledge the new chain:

```bash
oc annotate timestampauthority <name> rhtas.redhat.com/refresh-trust-material=true --overwrite -n <namespace>
```

## Related

- [Fulcio KMS Signer](fulcio-kms.md) — KMS signer for the Fulcio CA
- [Rekor KMS Signer](rekor-kms.md) — KMS signer for the Rekor transparency log
- [Timestamp Authority Key Rotation](tsa-key-rotation.md) — rotating the certificate chain and signer keys
- [FIPS](fips.md) — only the certificate chain is validated in KMS mode; KMS key FIPS compliance is the user's responsibility
