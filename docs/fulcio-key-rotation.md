# Fulcio Certificate rotation

This document provides detailed instructions on how to rotate the certificate used for the Fulcio service. The steps will vary depending on how you have the certificate configured. The following points apply to all configurations:

1. You can find the previous certificate/certificates in secrets with a prefix of `fulcio-cert-*`.
2. The Certificate currently used by the Fulcio service is available at `.status.certificateChain` on the Fulcio resource.
3. The new certificate will be automatically propagated to the root certificates for CTLOG.
4. The new certificate must be manually added to the TUF targets/ directory and the targets.json file, same as for the other services — see the "Update TUF Service" step below.

## Prerequisites
Before you begin, ensure that:

1. You have the necessary access to your Kubernetes cluster.
2. An instance of the Fulcio Service is running.

# Operator-Generated Private keys and Certificate
If you have deployed the operator with the default configuration found [here](https://github.com/securesign/secure-sign-operator/blob/main/config/samples/rhtas_v1_securesign.yaml), rotating the private keys and certificate is a straightforward process.
Remove the Fulcio resource:
```
oc delete fulcio <securesign_name> -n <namespace>
```
The operator will then automatically generate a new set of private keys and a new certificate, as well as redeploy the Fulcio Service.

# User-Created Keys and Certificate Chain
If you have deployed the Fulcio Service with a user-provided private key and certificate chain, you can follow these steps to rotate them. `signer.file.privateKeyRef` and `signer.certificateChain.certificateChainRef` must always be provided together — the operator does not support generating a certificate for a user-provided private key, or vice versa.
1. Generate a new private key for the certificate.
2. Create a new Kubernetes secret for the rotated key, password and Certificate.
3. Patch the securesign resource with updated references to the rotated key and certificate:
    ```
    signer:
      type: file
      certificateChain:
        organizationName: Red Hat
        certificateChainRef:
          name: rotated-cert
          key: rotated-cert
      file:
        privateKeyRef:
          name: rotated-private-key
          key: rotated-private-key
        privateKeyPasswordRef:
          name: rotated-private-key-pass
          key: rotated-private-key-pass
    ```
4. After patching, you should see the operator reconcile the Fulcio and CTLOG resources with the updated private key.

# Confirm the New Certificate

For all of the scenarios above, the operator requires confirmation before switching to a new certificate it sees
running on Fulcio:

```bash
kubectl annotate fulcio <name> rhtas.redhat.com/refresh-trust-material=true --overwrite -n <namespace>
```

The operator picks up the new certificate shortly after and removes the annotation on its own. To confirm it
went through, check that this comes back `True`:

```bash
kubectl get fulcio <name> -o jsonpath='{.status.conditions[?(@.type=="TrustMaterialAvailable")].status}' -n <namespace>
```

# Update TUF Service

Follow the [TUF key rotation documentation](TODO) to add the new certificate into the TUF service.
