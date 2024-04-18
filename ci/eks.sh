#!/bin/bash
# Ask for AWS account ID if its not stored in an environment variable ACCT
if [ -z "$ACCT" ]; then
  echo "Enter your AWS account ID: "
  read ACCT
fi
# Ask for the AWS access key if its not stored in an environment variable AWS_ACCESS_KEY_ID
if [ -z "$AWS_ACCESS_KEY" ]; then
  echo "Enter your AWS access key"
  read -s AWS_ACCESS_KEY
fi
# Ask for the AWS secret key if its not stored in an environment variable AWS_SECRET_ACCESS_KEY
if [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
  echo "Enter your AWS secret key: "
    read -s AWS_SECRET_ACCESS_KEY
fi

# Ask for the Wildcard DNS if its not stored in an environment variable WILDCARD_DNS
if [ -z "$WILDCARD_DNS" ]; then
  echo "Enter your wildcard DNS(NOTE: Do not include the *). Example (eks.octo-emerging.redhataicoe.com)":
  read WILDCARD_DNS
fi

# Fail if eksctl is not installed
if ! [ -x "$(command -v eksctl)" ]; then
  echo "eksctl is not installed. Please install eksctl and try again."
  exit 1
fi

# Ask for an email address to be used for the Let's Encrypt certificate
echo "Enter your email address to be used for the Let's Encrypt certificate: "
read EMAIL

# Deploy EKS cluster
eksctl create cluster --alb-ingress-access --external-dns-access --name rhtas-eks --nodes 4 --zones us-east-2b,us-east-2c --node-zones=us-east-2b --region us-east-2
eksctl utils associate-iam-oidc-provider --region=us-east-2 --cluster=rhtas-eks --approve
eksctl create iamserviceaccount --region us-east-2 --name ebs-csi-controller-sa --namespace kube-system --cluster rhtas-eks --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy --approve --role-only --role-name AmazonEKS_EBS_CSI_DriverRole
eksctl create addon --name aws-ebs-csi-driver --cluster rhtas-eks --service-account-role-arn arn:aws:iam::$ACCT:role/AmazonEKS_EBS_CSI_DriverRole --force


# Deploy nginx ingress
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/aws/deploy.yaml
kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s
kubectl patch ingressclass nginx --type=json -p='[{"op": "add", "path": "/metadata/annotations/ingressclass.kubernetes.io~1is-default-class", "value": "true"}]'


# Deploy OLM
kubectl create -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.25.0/crds.yaml
kubectl create -f https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.25.0/olm.yaml

# Record the value of the load balancer SVC
LB=$(kubectl get svc -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
echo "Load balancer: $LB"

# Wait for the user to update the DNS record by forcing them to press enter
read -p "Update the DNS record to point to $LB and press enter to continue"

# Deploy cert-manager
kubectl create -f https://operatorhub.io/install/cert-manager.yaml
bash -c 'until [ ! -z "$(oc get deployment cert-manager -n operators 2>/dev/null)" ]; do echo "Waiting for $i deployment to be created."; oc get pods -n operators; sleep 3; done'
kubectl wait --for=condition=available deployment/cert-manager -n operators --timeout=120s
bash -c 'until [ ! -z "$(oc get deployment cert-manager-webhook -n operators 2>/dev/null)" ]; do echo "Waiting for $i deployment to be created."; oc get pods -n operators; sleep 3; done'
kubectl wait --for=condition=available deployment/cert-manager-webhook -n operators --timeout=120s

# Define the cert-manager issuer
kubectl create secret -n operators generic acme-route53 --from-literal=secret-access-key=${AWS_SECRET_ACCESS_KEY}
cat <<EOF > issuer.yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-production
  namespace: operators
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ${EMAIL}
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - dns01:
        route53:
          region: us-east-2
          accessKeyID: ${AWS_ACCESS_KEY}
          secretAccessKeySecretRef:
            name: acme-route53
            key: secret-access-key
EOF

cat <<EOF > certificate.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ingress-certificate
  namespace: ingress-nginx
spec:
  secretName: ingress-certificate
  commonName: "*.${WILDCARD_DNS}"
  issuerRef:
    name: letsencrypt-production
    kind: ClusterIssuer
  dnsNames:
  - "*.${WILDCARD_DNS}"
EOF

kubectl create -f ./clusterissuer.yaml
kubectl create -f ./certificate.yaml
kubectl patch deployment ingress-nginx-controller -n ingress-nginx --type=json -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--default-ssl-certificate=ingress-nginx/ingress-certificate"}]'

# Deploy the operator
# TODO: Use operator-sdk to deploy the GA bundle
IMG=quay.io/redhat-user-workloads/rhtas-tenant/operator/rhtas-operator@sha256:584b4e47603e17c45d4b2a3bf7c71e9a4d249e2f50619fa1e6a57b6742d2e2ad make deploy

# Inform the user that the environment is ready to deploy TAS
echo "The environment is now setup and ready for RHTAS to be deployed"
echo "For deploying RHTAS, the following values can be used to define the deployment:"
echo "
  rekor:
    externalAccess:
      enabled: true
      host: rekor-server-securesign.${WILDCARD_DNS}
      ...
  fulcio:
    externalAccess:
      enabled: true
      host: fulcio-server-securesign.${WILDCARD_DNS}
      ...
  tuf:
    externalAccess:
      enabled: true
      host: tuf-securesign.${WILDCARD_DNS}
      ..."
