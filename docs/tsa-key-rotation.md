# Rotating the Certificate Chain and Signer Keys for the Timestamp Authority Service

This document provides detailed instructions on how to rotate the certificate chain and signer keys for the Timestamp Authority Service. The steps will vary depending on the type of signer in use: File-based, KMS, or Tink. The following points apply to all types:

1. To verify images signed before the rotation, you can find the previous certificate chain in secrets with a prefix of tsa-signer-config-*.
2. The certificate chain currently used by the Timestamp Authority Service will have the label rhtas.redhat.com/tsa.certchain.pem: certificateChain.
3. The new certificate chain will be propagated to the TUF targets/ directory and the targets.json file.

## Prerequisites
Before you begin, ensure that:

1. You have the necessary access to your Kubernetes cluster.
2. An instance of the Timestamp Authority Service is running.

# File-based Signer
## Operator-Generated Signer Keys and Certificate Chain
If you have deployed the operator with the default configuration found [here](https://github.com/securesign/secure-sign-operator/blob/fc9c5b01a487c263033faf6599467f8a676c412c/config/samples/rhtas_v1alpha1_securesign.yaml#L51), rotating the keys is a straightforward process. Simply delete the Timestamp Authority instance using the following command:
    ```
    oc delete timestampAuthority <securesign_name> -n <namespace>
    ```
The operator will then automatically generate a new set of keys and a new certificate chain, as well as redeploy the Timestamp Authority Service.

## Operator-Generated Certificate Chain
If you have deployed the Timestamp Authority Service with self-generated private keys for the root CA, intermediate CAs, and leaf CAs, follow these steps to rotate the keys:

1. Create a new Kubernetes secret for each of the rotated keys using the following command:
    ```
    oc create secret generic rotated-key -n <namespace> --from-file=rotated-key=<path/to/rotated/key>
    ```
2. Patch the securesign resource with updated references to the rotated keys:
    ```
    signer:
      certificateChain:
        rootCA:
          organizationName: Red Hat
          privateKeyRef:
            name: rotated-root-key
            key: rotated-root-key
        intermediateCA:
          - organizationName: Red Hat
            privateKeyRef:
              name: rotated-inter-key
              key: rotated-inter-key
        leafCA:
          organizationName: Red Hat
          privateKeyRef:
            name: rotated-leaf-key
            key: rotated-leaf-key
    ```
3. After patching the securesign resource, you should see the Timestamp Authority Service and the TUF service redeployed with the new certificate chain and private keys.

## User-Created Keys and Certificate Chain
If you have deployed the Timestamp Authority Service with a self-generated certificate chain and signer keys, the process is similar to the above:
1. Create a new secret for the signer key and certificate chain.
2. Patch the securesign resource with updated references to the rotated keys and certificate chain:
    ```
      signer:
        certificateChain:
          certificateChainRef:
            name: rotated-cert-chain
            key: rotated-cert-chain
        file:
          privateKeyRef:
            name: rotated-signer-key
            key: rotated-signer-key
    ```
3. After patching the securesign resource, you should see the Timestamp Authority Service and the TUF service redeployed with the new certificate chain and private keys.

# KMS (Key Management Service)
If you have deployed the Timestamp Authority Service using a KMS provider following these [steps](https://github.com/securesign/timestamp-authority?tab=readme-ov-file#cloud-kms), the process for rotating the keys is similar to the above:

1. Generate new keys and certificates using your KMS provider.
2. Fetch the new certificate chain using the fetch-tsa-certs binary, which can be found in command line tools.
3. Create a new secret for the certificate chain.
4. Patch the securesign resource with updated references to the rotated keys and certificate chain:
    ```
    signer:
      certificateChain:
        certificateChainRef:
          name: rotated-cert-chain
          key: rotated-cert-chain
      kms:
        keyResource: gcpkms://<new-key-resource>
    ```
5. After patching the securesign resource, you should see the Timestamp Authority Service and the TUF service redeployed with the new certificate chain and private keys.

# Tink
If you have deployed the Timestamp Authority Service using the Tink signer, following these [steps](https://github.com/securesign/timestamp-authority?tab=readme-ov-file#tink), the process for rotating the keys is similar to the previous methods:

1. Generate new keys and certificates using your KMS provider.
2. Generate a new Tink keyset using [Tinkey](https://developers.google.com/tink/tinkey-overview#installation).
3. Fetch the new certificate chain using the fetch-tsa-certs binary, which can be found in command line tools.
4. Create a new secret for the certificate chain.
5. Patch the securesign resource with updated references to the rotated keys and certificate chain:
    ```
    signer:
      certificateChain:
        certificateChainRef:
          name: rotated-cert-chain
          key: rotated-cert-chain
      tink:
        keyResource: gcp-kms://<new-key-resource>
        keysetRef:
          name: rotated-key-set
          key: rotated-key-set
    ```
6. After patching the securesign resource, you should see the Timestamp Authority Service and the TUF service redeployed with the new certificate chain and private keys.
