default install plus sign and verify



cosign sign --oidc-client-id trusted-artifact-signer quay.io/tdalton/rhtastest:ctlogtest40

cosign verify --certificate-identity=jdoe@redhat.com --certificate-oidc-issuer="https://keycloak-keycloak-system.apps.rosa.a5zfz-b4iz3-irx.3imj.p3.openshiftapps.com/realms/trusted-artifact-signer" quay.io/tdalton/rhtastest:ctlogtest40