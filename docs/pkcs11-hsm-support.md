# PKCS#11 / HSM Signer Support

Both Fulcio and CTLog support PKCS#11 hardware security modules (HSMs) as a
signer backend. The operator is vendor-agnostic: you provision the HSM token
and library via init containers and volumes, then point the CR at the
resulting artifacts.

## Prerequisites

- An HSM token initialized with a signing key (e.g. SoftHSM, Thales Luna,
  AWS CloudHSM).
- A PVC or CSI volume holding the token data (for persistence across pod
  restarts).
- The PKCS#11 `.so` module available in a container image.

## Example: Dual PKCS#11 (Fulcio + CTLog with SoftHSM)

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Securesign
metadata:
  name: example
spec:
  fulcio:
    ingress:
      enabled: true
    config:
      oidcIssuers:
        - clientID: "trusted-artifact-signer"
          issuerURL: "https://keycloak.example.com/realms/sigstore"
          issuer: "https://keycloak.example.com/realms/sigstore"
          type: "email"
    signer:
      type: pkcs11
      pkcs11:
        configRef:
          name: fulcio-pkcs11-config    # Secret with crypto11.conf
          key: crypto11.conf
        keyID: 99                       # CKA_ID of the CA root key
        keyLabel: PKCS11CA              # CKA_LABEL
      certificateChain:
        certificateChainRef:
          name: fulcio-root-ca          # Secret with the CA cert PEM
          key: cert.pem
    auth:
      env:
        - name: SOFTHSM2_CONF
          value: /etc/softhsm/softhsm2.conf
    initContainers:
      - name: hsm-lib-export
        image: my-hsm-vendor:latest
        command: ["cp", "/usr/lib64/pkcs11/libsofthsm2.so", "/var/run/hsm-lib/"]
        volumeMounts:
          - name: hsm-lib
            mountPath: /var/run/hsm-lib
    volumes:
      - name: softhsm-config
        configMap:
          name: softhsm-config
      - name: hsm-tokens
        persistentVolumeClaim:
          claimName: hsm-tokens-pvc
    volumeMounts:
      - name: softhsm-config
        mountPath: /etc/softhsm
        readOnly: true

  ctlog:
    logs:
      - prefix: trusted-artifact-signer
        active: true
        signer:
          type: pkcs11
          pkcs11:
            modulePath: /usr/lib64/pkcs11/libsofthsm2.so
            tokenLabel: PKCS11CA
            pinSecretRef:
              name: hsm-credentials
              key: pin
            publicKeyRef:
              name: ctlog-public-key
              key: public.pem
    auth:
      env:
        - name: SOFTHSM2_CONF
          value: /etc/softhsm/softhsm2.conf
    initContainers:
      - name: hsm-lib-export
        image: my-hsm-vendor:latest
        command: ["cp", "/usr/lib64/pkcs11/libsofthsm2.so", "/var/run/hsm-lib/"]
        volumeMounts:
          - name: hsm-lib
            mountPath: /var/run/hsm-lib
    volumes:
      - name: softhsm-config
        configMap:
          name: softhsm-config
      - name: hsm-tokens
        persistentVolumeClaim:
          claimName: ctlog-hsm-tokens-pvc
    volumeMounts:
      - name: softhsm-config
        mountPath: /etc/softhsm
        readOnly: true

  rekor:
    ingress:
      enabled: true
    signer:
      kms: secret
  trillian:
    database:
      create: true
  tuf:
    ingress:
      enabled: true
```

## Volume naming

The operator manages these volume names in PKCS#11 mode:

| Volume | Purpose | Source |
|--------|---------|-------|
| `hsm-tokens` | HSM token data | User-defined (PVC recommended) or EmptyDir default |
| `hsm-lib` | PKCS#11 `.so` module | Operator-managed EmptyDir |
| `pkcs11-config` | Fulcio crypto11.conf | Operator-managed Secret (from `configRef`) |
| `fulcio-pkcs11-cert` | Fulcio CA cert | Operator-managed Secret (from `certificateChainRef`) |

If you define a volume named `hsm-tokens` in `spec.volumes` (e.g. with a PVC
for persistence), the operator preserves your source. Otherwise it defaults to
EmptyDir (token data lost on pod restart).

The `auth` volume name is reserved by the operator for projected secret mounts
from `signer.auth.secretMount`.

## CTLog key selection

CTLog uses trillian's `keyspb.PKCS11Config` which identifies the token by
`tokenLabel` only. Key selection is handled at the token level (one signing key
per token), unlike Fulcio which supports per-key selection via `keyID`
and `keyLabel`.
