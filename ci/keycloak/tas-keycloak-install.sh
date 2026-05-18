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
        pod_name=$(oc get pod -n "$namespace" | grep "$pod_name_prefix" | grep "Running" | awk '{print $1}')
        if [ -n "$pod_name" ]; then
            pod_status=$(oc get pod -n "$namespace" "$pod_name" -o jsonpath='{.status.phase}')
            if [ "$pod_status" == "Running" ]; then
                echo "$pod_name is up and running in namespace $namespace."
                return 0
            else
                echo "$pod_name is in state: $pod_status. Retrying in $sleep_interval seconds..."
            fi
        else
            echo "No pods with the prefix '$pod_name_prefix' found in namespace $namespace. Retrying in $sleep_interval seconds..."
        fi

        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    echo "Timed out. No pods with the prefix '$pod_name_prefix' reached the 'Running' state within the specified time."
    return 1
}

log_realm_import_progress() {
    local namespace="$1"
    local realm_name="$2"
    local cli="$3"

    local started_msg has_errors_msg
    started_msg=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" \
        -o jsonpath='{.status.conditions[?(@.type=="Started")].message}' 2>/dev/null)
    has_errors_msg=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" \
        -o jsonpath='{.status.conditions[?(@.type=="HasErrors")].message}' 2>/dev/null)
    echo "  Started: ${started_msg:-unknown}"
    if [ -n "$has_errors_msg" ]; then
        echo "  HasErrors: $has_errors_msg"
    fi
    echo "  Import job:"
    $cli get job "$realm_name" -n "$namespace" -o custom-columns=NAME:.metadata.name,ACTIVE:.status.active,SUCCEEDED:.status.succeeded,FAILED:.status.failed 2>/dev/null \
        || echo "    (job not created yet)"
    echo "  Import pods:"
    $cli get pods -n "$namespace" -l app=keycloak-realm-import -o wide 2>/dev/null \
        || $cli get pods -n "$namespace" --field-selector=status.phase=Running 2>/dev/null | grep "$realm_name" || true
}

wait_for_realm_import_job() {
    local namespace="$1"
    local realm_name="$2"
    local timeout="${3:-900s}"
    local cli="${4:-kubectl}"

    local attempts=0
    while [[ $attempts -lt 30 ]]; do
        if $cli get job "$realm_name" -n "$namespace" >/dev/null 2>&1; then
            echo "Waiting for realm import Job '$realm_name' to complete (timeout: $timeout) ..."
            if $cli wait --for=condition=complete "job/${realm_name}" -n "$namespace" --timeout="$timeout"; then
                return 0
            fi
            if $cli wait --for=condition=failed "job/${realm_name}" -n "$namespace" --timeout=5s 2>/dev/null; then
                echo "Realm import Job '$realm_name' failed."
                return 1
            fi
            return 1
        fi
        sleep 2
        attempts=$((attempts + 1))
    done
    echo "Realm import Job '$realm_name' was not created."
    return 1
}

wait_for_realm_import() {
    local namespace="$1"
    local realm_name="${2:-trusted-artifact-signer-realm}"
    local limit="${3:-$max_attempts}"
    local cli="${4:-kubectl}"
    local job_timeout="${5:-900s}"
    local attempts=0
    local progress_interval=6
    local job_wait_started=false

    echo "Waiting for KeycloakRealmImport '$realm_name' to complete (up to $((limit * sleep_interval))s, job timeout: ${job_timeout}) ..."
    while [[ $attempts -lt $limit ]]; do
        status=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="Done")].status}' 2>/dev/null)
        if [ "$status" == "True" ]; then
            echo "KeycloakRealmImport '$realm_name' completed successfully."
            return 0
        fi
        has_errors=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="HasErrors")].status}' 2>/dev/null)
        if [ "$has_errors" == "True" ]; then
            echo "KeycloakRealmImport '$realm_name' reported errors."
            break
        fi

        if [ "$job_wait_started" != "true" ] && $cli get job "$realm_name" -n "$namespace" >/dev/null 2>&1; then
            job_wait_started=true
            echo "Realm import Job detected; waiting for Job completion ..."
            if wait_for_realm_import_job "$namespace" "$realm_name" "$job_timeout" "$cli"; then
                sleep 5
                status=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="Done")].status}' 2>/dev/null)
                if [ "$status" == "True" ]; then
                    echo "KeycloakRealmImport '$realm_name' completed successfully."
                    return 0
                fi
            fi
            has_errors=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="HasErrors")].status}' 2>/dev/null)
            if [ "$has_errors" == "True" ]; then
                break
            fi
        fi

        if [ $((attempts % progress_interval)) -eq 0 ]; then
            echo "Realm import not done yet (Done: ${status:-False})."
            log_realm_import_progress "$namespace" "$realm_name" "$cli"
        else
            echo "Realm import not done yet (Done: ${status:-False}). Retrying in $sleep_interval seconds..."
        fi
        sleep $sleep_interval
        attempts=$((attempts + 1))
    done

    echo "Timed out waiting for KeycloakRealmImport '$realm_name' to complete."
    log_realm_import_progress "$namespace" "$realm_name" "$cli"
    echo "--- KeycloakRealmImport status ---"
    $cli get keycloakrealmimport "$realm_name" -n "$namespace" -o yaml 2>/dev/null
    echo "--- Realm import job describe ---"
    $cli describe job "$realm_name" -n "$namespace" 2>/dev/null || echo "No realm import job found"
    echo "--- Realm import pod logs ---"
    $cli logs -n "$namespace" -l app=keycloak-realm-import --tail=100 2>/dev/null \
        || $cli logs -n "$namespace" --selector="job-name=${realm_name}" --tail=100 2>/dev/null \
        || echo "No realm import pod logs found"
    return 1
}

apply_realm_import_with_retry() {
    local namespace="$1"
    local realm_manifest="$2"
    local realm_name="${3:-trusted-artifact-signer-realm}"
    local limit="${4:-$max_attempts}"
    local cli="${5:-kubectl}"
    local job_timeout="${6:-900s}"
    local attempt
    local had_errors=false

    for attempt in 1 2; do
        $cli apply -f "$realm_manifest" -n "$namespace"
        if wait_for_realm_import "$namespace" "$realm_name" "$limit" "$cli" "$job_timeout"; then
            return 0
        fi
        has_errors=$($cli get keycloakrealmimport "$realm_name" -n "$namespace" -o jsonpath='{.status.conditions[?(@.type=="HasErrors")].status}' 2>/dev/null)
        if [ "$has_errors" == "True" ]; then
            had_errors=true
        fi
        if [ "$attempt" -eq 1 ] && [ "$had_errors" == "true" ]; then
            echo "Realm import reported errors; deleting KeycloakRealmImport and retrying once ..."
            $cli delete keycloakrealmimport "$realm_name" -n "$namespace" --ignore-not-found
            $cli delete job "$realm_name" -n "$namespace" --ignore-not-found
            wait_for_keycloak_pod_ready "$namespace" "120s" "$cli" || return 1
            had_errors=false
        else
            break
        fi
    done
    return 1
}

# RHBK configures readiness probes against /health/ready on port 9000. Waiting for the
# pod Ready condition is more reliable than curling the ingress route or port-forwarding.
wait_for_keycloak_pod_ready() {
    local namespace="$1"
    local timeout="${2:-600s}"
    local cli="${3:-kubectl}"

    echo "Waiting for Keycloak pod readiness (management /health/ready probe) ..."
    if ! $cli wait --for=condition=Ready pod \
        -l app=keycloak,app.kubernetes.io/managed-by=keycloak-operator \
        -n "$namespace" \
        --timeout="$timeout"; then
        echo "Timed out waiting for Keycloak pod to become Ready."
        $cli get pods -n "$namespace" -l app=keycloak,app.kubernetes.io/managed-by=keycloak-operator -o wide 2>/dev/null
        return 1
    fi
    echo "Keycloak pod is Ready."
    return 0
}

install_openshift_keycloak() {
    # Pipelines often run on slower clusters; allow override via env.
    local openshift_max_attempts="${KEYCLOAK_REALM_IMPORT_ATTEMPTS:-90}"
    local realm_import_job_timeout="${KEYCLOAK_REALM_IMPORT_JOB_TIMEOUT:-1200s}"

    BASE_DOMAIN=apps.$(oc get dns cluster -o jsonpath='{.spec.baseDomain}')
    echo "HOSTNAME=https://keycloak-keycloak-system.$BASE_DOMAIN" > ci/keycloak/resources/overlay/openshift/hostname.env

    oc apply --kustomize ci/keycloak/operator/overlay/openshift
    check_pod_status "keycloak-system" "rhbk-operator"
    if [ $? -ne 0 ]; then
        echo "Pod status check failed. Exiting the script."
        exit 1
    fi
    oc apply --kustomize ci/keycloak/resources/overlay/openshift
    echo "Waiting for PostgreSQL to become ready ..."
    if ! oc rollout status statefulset/postgresql-db -n keycloak-system --timeout=600s; then
        echo "PostgreSQL rollout failed. Exiting the script."
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

    if ! wait_for_keycloak_pod_ready "keycloak-system" "600s" oc; then
        echo "Keycloak pod readiness check failed. Exiting the script."
        exit 1
    fi

    if ! apply_realm_import_with_retry "keycloak-system" "ci/keycloak/resources/base/realm-import.yaml" \
        "trusted-artifact-signer-realm" "$openshift_max_attempts" oc "$realm_import_job_timeout"; then
        echo "Realm import failed. Exiting the script."
        exit 1
    fi
}

wait_for_ingress_nginx() {
    local timeout="${1:-300s}"
    echo "Waiting for ingress-nginx controller to be ready ..."
    if ! kubectl wait --namespace ingress-nginx \
        --for=condition=ready pod \
        --selector=app.kubernetes.io/component=controller \
        --timeout="$timeout"; then
        echo "ingress-nginx controller did not become ready in time."
        kubectl get pods -n ingress-nginx 2>/dev/null
        return 1
    fi
    echo "Waiting for ingress-nginx admission webhook jobs to complete ..."
    kubectl wait --namespace ingress-nginx \
        --for=condition=complete job \
        -l app.kubernetes.io/component=admission-webhook \
        --timeout=120s 2>/dev/null || true
    return 0
}

install_kind_keycloak() {
    KEYCLOAK_VERSION="${KEYCLOAK_VERSION:-26.5.3}"

    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.14.3/deploy/static/provider/kind/deploy.yaml
    wait_for_ingress_nginx "300s" || echo "Warning: ingress-nginx not ready yet (will retry before Ingress creation)."

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

    if ! wait_for_keycloak_pod_ready "keycloak-system" "300s" kubectl; then
        echo "Keycloak pod readiness check failed."
        return 1
    fi

    if ! apply_realm_import_with_retry "keycloak-system" "ci/keycloak/resources/base/realm-import.yaml"; then
        echo "Realm import failed."
        return 1
    fi

    if wait_for_ingress_nginx "300s"; then
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
    else
        echo "Warning: ingress-nginx not ready; Keycloak is available via keycloak-internal.keycloak-system.svc"
    fi
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
