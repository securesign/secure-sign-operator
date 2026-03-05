#!/usr/bin/env sh

# Define the maximum number of attempts and the sleep interval (in seconds)
max_attempts=30
sleep_interval=10

# Function to check pod status
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

# Install SSO Operator and Keycloak service
install_sso_keycloak() {
    oc apply --kustomize ci/keycloak/operator/base
    check_pod_status "keycloak-system" "rhsso-operator"
    # Check the return value from the function
    if [ $? -ne 0 ]; then
        echo "Pod status check failed. Exiting the script."
        exit 1
    fi
    oc apply --kustomize ci/keycloak/resources/base
    check_pod_status "keycloak-system" "keycloak-postgresql"
    # Check the return value from the function
    if [ $? -ne 0 ]; then
        echo "Pod status check failed. Exiting the script."
        exit 1
    fi
}

# Install Red Hat SSO Operator and setup Keycloak service
install_sso_keycloak
