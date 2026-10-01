#!/usr/bin/env sh

max_attempts=30
sleep_interval=10

usage() {
  echo "Usage: $0 [openshift|kind]"
  echo "  openshift -> install RHBK operator via OLM (OpenShift)"
  echo "  kind      -> install upstream Keycloak operator (Kind)"
}

check_pod_status() {
    local namespace="$1"
    local pod_name_prefix="$2"
    local attempts=0

    while [[ $attempts -lt $max_attempts ]]; do
        pods_output=$(oc get pods -n "$namespace" --no-headers 2>&1)
        pods_status=$?
        if [ $pods_status -ne 0 ]; then
            echo "Failed to list pods in namespace $namespace (exit code: $pods_status):"
            echo "$pods_output"
            sleep $sleep_interval
            attempts=$((attempts + 1))
            continue
        fi

        matching_pods=$(echo "$pods_output" | awk -v prefix="$pod_name_prefix" 'index($1, prefix) == 1')
        pod_name=$(echo "$matching_pods" | awk '$3 == "Running" {print $1; exit}')
        if [ -n "$pod_name" ]; then
            pod_status=$(oc get pod -n "$namespace" "$pod_name" -o jsonpath='{.status.phase}')
            if [ "$pod_status" == "Running" ]; then
                echo "$pod_name is up and running in namespace $namespace."
                return 0
            else
                echo "$pod_name is in state: $pod_status. Retrying in $sleep_interval seconds..."
            fi
        elif [ -n "$matching_pods" ]; then
            echo "Pods with the prefix '$pod_name_prefix' exist but none are Running:"
            echo "NAME READY STATUS RESTARTS AGE"
            echo "$matching_pods"
            echo "Retrying in $sleep_interval seconds..."
        else
            echo "No pods with the prefix '$pod_name_prefix' found in namespace $namespace. Retrying in $sleep_interval seconds..."
        fi

        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    echo "Timed out. No pods with the prefix '$pod_name_prefix' reached the 'Running' state within the specified time."
    return 1
}

dump_olm_diagnostics() {
    local namespace="$1"
    local subscription="$2"

    echo "--- OLM resources in namespace $namespace ---"
    oc get operatorgroup,subscription,installplan,csv,deployment,pods -n "$namespace" -o wide 2>&1 || true

    echo "--- Subscription $subscription ---"
    oc describe subscription "$subscription" -n "$namespace" 2>&1 || true

    echo "--- Subscription status conditions ---"
    oc get subscription "$subscription" -n "$namespace" \
        -o jsonpath='{range .status.conditions[*]}type={.type}{" status="}{.status}{" reason="}{.reason}{" message="}{.message}{"\n"}{end}' 2>&1 || true
    echo

    echo "--- InstallPlans ---"
    oc get installplan -n "$namespace" -o yaml 2>&1 || true

    echo "--- ClusterServiceVersions ---"
    oc get csv -n "$namespace" \
        -o custom-columns='NAME:.metadata.name,PHASE:.status.phase,REASON:.status.reason,MESSAGE:.status.message' 2>&1 || true

    echo "--- Pod details ---"
    oc describe pods -n "$namespace" 2>&1 || true

    echo "--- Operator logs ---"
    oc logs deployment/"$subscription" -n "$namespace" --all-containers --tail=100 2>&1 || true

    echo "--- Recent namespace events ---"
    oc get events -n "$namespace" --sort-by='.lastTimestamp' 2>&1 || true

    echo "--- redhat-operators CatalogSource ---"
    oc get catalogsource redhat-operators -n openshift-marketplace -o yaml 2>&1 || true

    echo "--- Marketplace catalog pods ---"
    oc get pods -n openshift-marketplace -o wide 2>&1 || true

    echo "--- Available channels for package $subscription ---"
    oc get packagemanifest "$subscription" -n openshift-marketplace \
        -o jsonpath='{range .status.channels[*]}{.name}{"\n"}{end}' 2>&1 || true
    echo
}

wait_for_realm_import() {
    local namespace="$1"
    local realm_name="${2:-trusted-artifact-signer-realm}"
    local limit="${3:-$max_attempts}"
    local attempts=0

    echo "Waiting for KeycloakRealmImport '$realm_name' to complete..."
    while [[ $attempts -lt $limit ]]; do
        status=$(kubectl get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="Done")].status}' 2>/dev/null)
        if [ "$status" == "True" ]; then
            echo "KeycloakRealmImport '$realm_name' completed successfully."
            return 0
        fi
        echo "Realm import not done yet (status: $status). Retrying in $sleep_interval seconds..."
        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    echo "Timed out waiting for KeycloakRealmImport '$realm_name' to complete."
    echo "--- KeycloakRealmImport status ---"
    kubectl get keycloakrealmimport "$realm_name" -n "$namespace" -o yaml 2>/dev/null
    echo "--- Realm import pod logs ---"
    kubectl logs -n "$namespace" -l app=keycloak-realm-import --tail=50 2>/dev/null || echo "No realm import pod logs found"
    return 1
}

install_openshift_keycloak() {
    local openshift_max_attempts=60

    BASE_DOMAIN=apps.$(oc get dns cluster -o jsonpath='{ .spec.baseDomain }')
    echo "HOSTNAME=https://keycloak-keycloak-system.$BASE_DOMAIN" > ci/keycloak/resources/overlay/openshift/hostname.env

    oc apply --kustomize ci/keycloak/operator/overlay/openshift
    check_pod_status "keycloak-system" "rhbk-operator"
    if [ $? -ne 0 ]; then
        dump_olm_diagnostics "keycloak-system" "rhbk-operator"
        echo "Pod status check failed. Exiting the script."
        exit 1
    fi
    oc apply --kustomize ci/keycloak/resources/overlay/openshift
    check_pod_status "keycloak-system" "postgresql-db"
    if [ $? -ne 0 ]; then
        echo "Pod status check failed. Exiting the script."
        exit 1
    fi

    local attempts=0
    while [[ $attempts -lt $openshift_max_attempts ]]; do
        status=$(oc get keycloaks/keycloak -n keycloak-system -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
        if [ "$status" == "True" ]; then
            echo "Keycloak CR is ready."
            break
        fi
        echo "Keycloak CR not ready yet (status: $status). Retrying in $sleep_interval seconds..."
        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    if [ "$status" != "True" ]; then
        echo "Timed out waiting for Keycloak CR to become ready."
        oc get keycloaks/keycloak -n keycloak-system -o yaml
        exit 1
    fi

    oc apply -f ci/keycloak/resources/base/realm-import.yaml -n keycloak-system
    wait_for_realm_import "keycloak-system" "trusted-artifact-signer-realm" "$openshift_max_attempts"
    if [ $? -ne 0 ]; then
        echo "Realm import failed. Exiting the script."
        exit 1
    fi
}

install_kind_keycloak() {
    KEYCLOAK_VERSION="${KEYCLOAK_VERSION:-26.5.3}"

    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.14.3/deploy/static/provider/kind/deploy.yaml
    kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s

    kubectl apply --kustomize ci/keycloak/operator/overlay/kind

    kubectl apply -f "https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/${KEYCLOAK_VERSION}/kubernetes/keycloaks.k8s.keycloak.org-v1.yml"
    kubectl apply -f "https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/${KEYCLOAK_VERSION}/kubernetes/keycloakrealmimports.k8s.keycloak.org-v1.yml"
    kubectl -n keycloak-system apply -f "https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/${KEYCLOAK_VERSION}/kubernetes/kubernetes.yml"

    kubectl patch clusterrolebinding keycloak-operator-clusterrole-binding \
      --type='json' -p='[{"op": "replace", "path": "/subjects/0/namespace", "value":"keycloak-system"}]'

    kubectl wait --for=condition=available deployment/keycloak-operator -n keycloak-system --timeout=120s

    kubectl apply --kustomize ci/keycloak/resources/overlay/kind

    kubectl rollout status statefulset/postgresql-db -n keycloak-system --timeout=120s

    local attempts=0
    while [[ $attempts -lt $max_attempts ]]; do
        status=$(kubectl get keycloaks/keycloak -n keycloak-system -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
        if [ "$status" == "True" ]; then
            echo "Keycloak is ready."
            break
        fi
        echo "Keycloak not ready yet (status: $status). Retrying in $sleep_interval seconds..."
        kubectl get pods -n keycloak-system
        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    if [ "$status" != "True" ]; then
        echo "Timed out waiting for Keycloak to become ready."
        return 1
    fi

    kubectl apply -f ci/keycloak/resources/base/realm-import.yaml -n keycloak-system
    wait_for_realm_import "keycloak-system"
    if [ $? -ne 0 ]; then
        echo "Realm import failed."
        return 1
    fi

    kubectl create -n keycloak-system -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: keycloak
spec:
  rules:
  - host: keycloak-internal.keycloak-system.svc
    http:
      paths:
      - backend:
          service:
            name: keycloak-internal
            port:
              number: 80
        path: /
        pathType: Prefix
EOF
}

choice="${1:-openshift}"
case "$choice" in
  openshift)
    install_openshift_keycloak
    ;;
  kind)
    install_kind_keycloak
    ;;
  -h|--help|help)
    usage
    ;;
esac
