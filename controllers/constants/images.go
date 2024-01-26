package constants

const (
	TrillianLogSignerImage = "registry.redhat.io/rhtas-tech-preview/trillian-logsigner-rhel9@sha256:3c60ec029bc6742d9e1a62f057b2c7da928d0b13c50985495a4670c5538310d3"
	TrillianServerImage    = "registry.redhat.io/rhtas-tech-preview/trillian-logserver-rhel9@sha256:5f9fcca2db9dbcbed0862d7a7e13cf355a3299624f0967836ea512c5b769ebb4"
	TrillianDbImage        = "registry.redhat.io/rhtas-tech-preview/trillian-database-rhel9@sha256:508ff03f1ba8bd337ef5986535841cdbecd946be482c58ba91f6fdb51c2e5f9e"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/rhtas-tech-preview/trillian-netcat-rhel9@sha256:a43e9a384050d398a73e90d51c9c0f9f1af426117fa9bf6725674de7a95f0873"

	FulcioServerImage = "registry.redhat.io/rhtas-tech-preview/fulcio-rhel9@sha256:12fac8e6d83056a7e5108cf92d6c622ef800ea0f2361e5b5d428a9a94811dd10"

	RekorRedisImage    = "docker.io/redis@sha256:6c42cce2871e8dc5fb3e843ed5c4e7939d312faf5e53ff0ff4ca955a7e0b2b39"
	RekorServerImage   = "registry.redhat.io/rhtas-tech-preview/rekor-server-rhel9@sha256:53b650ad487dce78025d1dbddc5f25116c132f4e78b7d6f8c1dd0638574f6db3"
	RekorSearchUiImage = "registry.redhat.io/rhtas-tech-preview/rekor-search-ui-rhel9@sha256:ea4344bc762809ca46ea0708de378d8592b97194a9c1e08eb9396294276818bf"

	TufImage = "registry.redhat.io/rhtas-tech-preview/tuf-server-rhel9@sha256:e61b455868b416882dc45fe53a5039077de9c932865361fde28d52b31e4a68d2"

	CTLogImage        = "registry.redhat.io/rhtas-tech-preview/ct-server-rhel9@sha256:17eafff9bc34610d0295654df5adcf6e090bca6581cfc0eb0bb4896405953ac2"
	ClientServerImage = "registry.redhat.io/rhtas-tech-preview/client-server-rhel9@sha256:60cdd00990d5372889a33cb93258b8dc026a9aa27c6f757bce25a500414d03b6"
)
