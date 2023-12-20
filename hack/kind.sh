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
