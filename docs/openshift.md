# Installing on OpenShift

## Install from OperatorHub (recommended)

1. In the OpenShift web console, navigate to **Operators > OperatorHub**
2. Search for **Red Hat Trusted Artifact Signer**
3. Click **Install** and accept the default settings:
   - **Update channel**: `stable`
   - **Installation mode**: All namespaces
   - **Installed Namespace**: `openshift-rhtas-operator`
4. Click **Install** and wait for the operator to reach **Succeeded** status

Alternatively, create the Subscription directly:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhtas-operator
  namespace: openshift-rhtas-operator
spec:
  channel: stable
  name: rhtas-operator
  source: redhat-operators
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
```

```sh
oc create namespace openshift-rhtas-operator
oc apply -f subscription.yaml
```

## Install via kustomize

For environments without OLM or for pinning to a specific version:

```sh
oc apply --kustomize https://github.com/securesign/secure-sign-operator/config/overlays/openshift?ref=<tag-or-branch>
```

Or clone the repository and apply locally:

```sh
git clone --branch <tag> https://github.com/securesign/secure-sign-operator.git
cd secure-sign-operator
oc apply --kustomize config/overlays/openshift
```

## Verify

```sh
oc get pods -n openshift-rhtas-operator
```

The operator pod should be `1/1 Running`. Prometheus scraping is configured automatically via a ServiceMonitor with service-serving-cert TLS.

## Uninstall

**OLM install**: Delete the Subscription and ClusterServiceVersion from the OpenShift web console, or:

```sh
oc delete subscription rhtas-operator -n openshift-rhtas-operator
oc delete csv -n openshift-rhtas-operator -l operators.coreos.com/rhtas-operator.openshift-rhtas-operator
```

**Kustomize install**:

```sh
oc delete --kustomize config/overlays/openshift
```
