kind delete cluster

make docker-build && make docker-push 

# spin up kind cluster
cat <<EOF | kind create cluster --image kindest/node:v1.28.0 --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - containerPath: /var/lib/kubelet/config.json
    hostPath: "${HOME}/.docker/config.json"
EOF

kind get kubeconfig > /tmp/config
chown $USER:$USER /tmp/config
if [[ -d ~/.kube ]] && [[ -f ~/.kube/config ]]
then
  export KUBECONFIG=~/.kube/config:/tmp/config
  oc config view --flatten > merged-config.yaml
  mv merged-config.yaml ~/.kube/config
else
  mv /tmp/config ~/.kube/config
fi
chmod go-r ~/.kube/config

oc config use-context kind-kind


#install OLM
kubectl create -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.25.0/crds.yaml
# wait for a while to be sure CRDs are installed
sleep 1
kubectl create -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.25.0/olm.yaml

#install keycloak from Kind overlay
kubectl create --kustomize ci/keycloak/operator/overlay/kind
until [ ! -z "$(kubectl get pod -l name=keycloak-operator -n keycloak-system 2>/dev/null)" ]
do
  echo "Waiting for keycloak operator. Pods in keycloak-system namespace:"
  kubectl get pods -n keycloak-system
  sleep 10
done
kubectl create --kustomize ci/keycloak/resources/overlay/kind
until [[ $( oc get keycloak keycloak -o jsonpath='{.status.ready}' -n keycloak-system 2>/dev/null) == "true" ]]
do
  printf "Waiting for keycloak deployment. \n Keycloak ready: %s \n" $(oc get keycloak keycloak -o jsonpath='{.status.ready}' -n keycloak-system)
  sleep 10
done

make deploy && kubectl create -f config/samples/rhtas_v1alpha1_securesign.yaml && sleep 30 && kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-ctlog-system ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-fulcio-system ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-rekor-system ;kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-rekor-system ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-trillian-system ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-trillian-system ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-trusted-artifact-signer-clientserver ; kubectl create secret generic pull-secret --from-file=.dockerconfigjson=/tmp/pull-secret.txt --type=kubernetes.io/dockerconfigjson -n securesign-sample-tuf-system

