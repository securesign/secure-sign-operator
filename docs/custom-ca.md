# Custom CA Bundle

Configuring deployments to trust custom Certificate Authorities (CAs) or self-signed certificates is often necessary for
ensuring secure communication between components or external OIDC service. This guide provides instructions how to add
custom CA bundle for managed operands.

## Prerequisites: ConfigMap with the CA Bundle

Before configuring an operand, you need to create a ConfigMap that includes your CA bundle in the same namespace where
the application will be deployed. You can achieve this using one of the following methods:

1. **Manually Create a ConfigMap**:

   Create a ConfigMap manually by following the [Kubernetes documentation](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/).

   Example:
   ```yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: custom-ca-bundle
   data:
     ca-bundle.crt: |
       -----BEGIN CERTIFICATE-----
       MIIC... (certificate content)
       -----END CERTIFICATE-----
   ```

2. **Use Cert-Manager**:

   Cert-Manager can generate and manage trusted CA bundles. Follow the [Cert-Manager Documentation](https://cert-manager.io/docs/usage/certificate/) for detailed steps.

3. **Use OpenShift's Feature to Inject Trusted CA Bundles**:

   OpenShift can automatically inject trusted CA bundles into a ConfigMap. Follow the [OpenShift Documentation](https://docs.openshift.com/container-platform/latest/networking/configuring-a-custom-pki.html#certificate-injection-using-operators_configuring-a-custom-pki) for detailed steps.

## Configure operand to use Custom CA Bundle

This is done by adding an annotation to the relevant Custom Resource Definitions (CRDs). It can be added to any CRD that
the operator manages (Securesign, Trillian, Fulcio, Rekor, CTlog). The annotation key is `rhtas.redhat.com/trusted-ca`,
and the value should be the name of the ConfigMap created in the prerequisites.

### Example on a Securesign

```yaml
apiVersion: rhtas.redhat.com/v1alpha1
kind: Securesign
metadata:
  name: example-instance
  annotations:
    rhtas.redhat.com/trusted-ca: "name-of-your-configmap"
spec:
  # other specifications
```
