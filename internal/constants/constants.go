package constants

const (
	AppName = "trusted-artifact-signer"

	ReadyCondition = "Ready"
	ReasonResolved = "Resolved"

	SecretMountPath = "/var/run/secrets/tas"

	KeyPrivate  = "private"
	KeyPublic   = "public"
	KeyCert     = "cert"
	KeyPassword = "password"

	HealthzPath = "/healthz"

	HSMTokensVolumeName = "hsm-tokens"
	HSMLibVolumeName    = "hsm-lib"
	HSMTokensMountPath  = "/var/run/hsm-tokens"
	HSMLibMountPath     = "/var/run/hsm-lib"
)
