package constants

const (
	AppName = "trusted-artifact-signer"

	ReadyCondition = "Ready"

	SecretMountPath = "/var/run/secrets/tas"

	KeyPrivate  = "private"
	KeyPublic   = "public"
	KeyCert     = "cert"
	KeyPassword = "password"

	HealthzPath = "/healthz"
)
