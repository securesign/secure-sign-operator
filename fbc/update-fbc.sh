get_latest_push_operator_images(){
    # expects you are logged in
    operator_snapshot=$(kubectl get snapshots -l appstudio.openshift.io/application=operator,pac.test.appstudio.openshift.io/event-type=push --sort-by=.metadata.creationTimestamp --no-headers | tail -n 1 | awk '{print $1}')
    images=($(kubectl get snapshot $operator_snapshot -o yaml | yq '.spec.components[].containerImage'))
    bundle_image=""
    operator_image=""
    if [[ "${images[0]}" == *"rhtas-operator-bundle"* ]]; then
        bundle_image="${images[0]}"
        operator_image="${images[1]}"
    elif [[ "${images[1]}" == *"rhtas-operator-bundle"* ]]; then
        operator_image="${images[0]}"
        bundle_image="${images[1]}"
    fi
    if [[ -z $bundle_image ]]; then
        echo "Could not get the bundle image. This is used to in the opm command, and thus is required. Failure."
        return 1
    fi
    echo $bundle_image $operator_image
    return 0
}

test=$(get_latest_push_operator_images)
echo $test