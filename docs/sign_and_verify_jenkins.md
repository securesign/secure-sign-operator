# Signing and Verifying Artifacts in Jenkins with RHTAS

## Prerequisites
* Jenkins Version: Ensure Jenkins v2.440.2 or newer is installed.
* RHTAS Installation: RHTAS should be installed using either the Operator or Helm Charts. This setup requires an OpenID Connect (OIDC) provider for authentication. For this guide, Keycloak is used as the OIDC provider.


### Configuring Jenkins
To ensure the pipeline operates smoothly, the following Jenkins configurations are necessary:

1) Install the [Docker Pipeline Plugin](https://plugins.jenkins.io/docker-workflow/): Verify that the Docker Pipeline plugin is installed and enabled.
2) Install the [Docker API Plugin](https://plugins.jenkins.io/docker-java-api/): Ensure the Docker API Plugin is installed and enabled.
3) Install the [Docker Plugin](https://plugins.jenkins.io/docker-plugin/): Ensure the Docker Plugin is installed and enabled.
4) Install the [Credentials Plugin](https://plugins.jenkins.io/credentials/): Make sure the Credentials plugin is installed and enabled.

#### Setting Up Credentials
Credentials will need to be set using the credentials plugin. There are three credentials in total that will need to be configured.

* Red Hat Registry Credentials (For the cosign image)
    * Navigate to Dashboard > Manage Jenkins > Credentials > (Global Domain) > Add Credentials.
    * Select `Username with Password`.
    * Enter your Red Hat username and password or create a Service Account to use instead.
    * Set an ID for these credentials; for ease, you can use `redhat_io_credentials` as the default ID. 

* Cosign Credentials
    * During the cosign signing process, cosign pushes the signed image to an image registry. As a result, you will need to provide credentials for the image repo you intend to use, to do this please follow the steps below:
        * Navigate to Dashboard > Manage Jenkins > Credentials > (Global Domain) > Add Credentials.
        * Select `Secret text`.
        * Enter your image repository credentials or create a Service Account to use instead.
        * Two secrets should be created here, username and password which default to `img-repo-username` and `img-repo-password` respectively.

* OIDC Password
    * A password for the OIDC provider will also need to be configured using the credentials plugin (ID is defaulted to `oidc-password`). This should be setup in a similar fashion to the above, and created as a `Secret text`


#### Setting Up A Pipeline
To Create a pipeline, select Dashboard > New item > Pipeline. Pipelines can be created using scm (Source Control Management), but in this example, we use a simple pipeline script that signs and verify's an image. An example can be found below.
```
pipeline {
    agent none
    environment {
        HOME = "${env.WORKSPACE}"
        IMG_REPO = "quay.io"
        IMG_REPO_USERNAME = credentials('img-repo-username')
        IMG_REPO_PASSWORD = credentials('img-repo-password')
        IMG = "<repo>/<path>/<image_name>"
        TAG = "latest"
        TUF_URL = "https://tuf-url.com"
        FULCIO_URL = "https://fulcio-server-url.com"
        REKOR_URL = "https://rekor-server-url.com"
        OIDC_ISSUER_URL = "https://keycloak-url.com/auth/realms/client_id"
        OIDC_CLIENT_ID = "client_id"
        OIDC_USERNAME = "oidc_username"
        OIDC_PASSWORD = credentials('oidc-password')
    }
    stages {
        stage("Sign & Verify Image") {
            agent {
                docker { 
                    image 'registry.redhat.io/rhtas/cosign-rhel9:1.0.0'
                    registryUrl 'https://registry.redhat.io'
                    registryCredentialsId 'redhat_io_credentials'
                }
            }
            steps {
                script {
                    sh """
                    cosign login ${env.IMG_REPO} -u \$IMG_REPO_USERNAME -p \$IMG_REPO_PASSWORD
                    cosign initialize --mirror=${env.TUF_URL} --root=${env.TUF_URL}/root.json
                    curl -X POST -H "Content-Type: application/x-www-form-urlencoded" \
                            -d "username=${env.OIDC_USERNAME}" \
                            -d "password=\$OIDC_PASSWORD" \
                            -d "grant_type=password" \
                            -d "scope=openid" \
                            -d "client_id=${env.OIDC_CLIENT_ID}" \
                            ${env.OIDC_ISSUER_URL}/protocol/openid-connect/token | sed -E 's/.*"id_token":"([^"]*)".*/\\1/' > token.txt
                    cosign sign -y --fulcio-url=${env.FULCIO_URL} --rekor-url=${env.REKOR_URL} --oidc-issuer=${env.OIDC_ISSUER_URL} --oidc-client-id=${env.OIDC_CLIENT_ID} --identity-token=token.txt ${env.IMG}:${env.TAG}
                    cosign verify --rekor-url=${env.REKOR_URL} --certificate-identity ${env.OIDC_USERNAME} --certificate-oidc-issuer ${env.OIDC_ISSUER_URL} ${env.IMG}:${env.TAG}
                    """
                }
            }
        }
    }
}
```
