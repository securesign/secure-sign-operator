# Fulcio Certificate rotation

This document provides detailed instructions on how to rotate the certificate used for the Fulcio service. The steps will vary depending on how you have the certificate configured. The following points apply to all configurations:

1. You can find the previous certificate/certificates in secrets with a prefix of `fulcio-cert-*`.
1. The Certificate currently use by fulcio service will have the label `rhtas.redhat.com/fulcio_v1.crt.pem: cert`
2. The new certificate will be propagated to the TUF targets/ directory and the targets.json file.
3. The new certificate will be propagated to the root certificates for CTLOG.

## Prerequisites
Before you begin, ensure that:

1. You have the necessary access to your Kubernetes cluster.
2. An instance of the Fulcio Service is running.

# Operator-Generated Private keys and Certificate
If you have deployed the operator with the default configuration found [here](https://github.com/securesign/secure-sign-operator/blob/fc9c5b01a487c263033faf6599467f8a676c412c/config/samples/rhtas_v1alpha1_securesign.yaml#L29), rotating the private keys and certificate is a straightforward process. Simply delete the Fulcio instance using the following command:
    ```
    oc delete fulcio <securesign_name> -n <namespace>
    ```
The operator will then automatically generate a new set of private keys and a new certificate, as well as redeploy the Fulcio Service.

# Operator-Generated Certificate
If you have deployed the Fulcio Service with self-generated private keys, you can follow these steps to rotate the keys:
1. Generate a new private key.
2. Create a new Kubernetes secret for the rotated key and the key's password using the following commands:
    ```
    oc create secret generic <secret_name> -n <namespace> --from-file=<key>=<path/to/private/key>
    oc create secret generic <secret_name> -n <namespace> --from-literal=<key>=<password>  
    ```
3. Patch the securesign resource with updated references to the rotated keys:
    ```
    certificate:
      organizationName: Red Hat
      privateKeyRef:
        name: rotated-private-key
        key: rotated-private-key
      privateKeyPasswordRef:
        name: rotated-private-key-pass
        key: rotated-private-key-pass
    ```
4. After patching, you should see the operator reconcile the Fulcio, CTLOG and TUF resources with the updated private key

# User-Created Keys and Certificate Chain
If you have deployed the Fulcio Service with self-generated private keys and a self generated Certificate, you can follow these steps to rotate the keys, this process is similar to the above:
1. Generate a new private key for the certificate.
2. Create a new Kubernetes secret for the rotated key, password and Certificate.
3. Patch the securesign resource with updated references to the rotated key and certificate:
    ```
    certificate:
      organizationName: Red Hat
      privateKeyRef:
        name: rotated-private-key
        key: rotated-private-key
      privateKeyPasswordRef:
        name: rotated-private-key-pass
        key: rotated-private-key-pass
      caRef:
        name: rotated-cert
        key: rotated-cert
    ```
4. After patching, you should see the operator reconcile the Fulcio, CTLOG and TUF resources with the updated private key.
