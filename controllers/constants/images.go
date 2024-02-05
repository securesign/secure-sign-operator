package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logsigner-1-5@sha256:3c60ec029bc6742d9e1a62f057b2c7da928d0b13c50985495a4670c5538310d3"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logserver-1-5@sha256:5f9fcca2db9dbcbed0862d7a7e13cf355a3299624f0967836ea512c5b769ebb4"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-database-1-5@sha256:e8e038bf1ca79f44a12b63b460f60148c9a230c2e551d13783626f03ce2573a1"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-netcat-1-5@sha256:a43e9a384050d398a73e90d51c9c0f9f1af426117fa9bf6725674de7a95f0873"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio-1-0-gamma/fulcio-1-0-gamma@sha256:12fac8e6d83056a7e5108cf92d6c622ef800ea0f2361e5b5d428a9a94811dd10"

	RekorRedisImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/redis-0-6@sha256:acf920baf6ee1715c0c9d7ddf69867d331c589d3afa680048c508943078d9585"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-server-1-0-gamma@sha256:53b650ad487dce78025d1dbddc5f25116c132f4e78b7d6f8c1dd0638574f6db3"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-search-ui-1-0-gamma@sha256:ea4344bc762809ca46ea0708de378d8592b97194a9c1e08eb9396294276818bf"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/tuf-server-0-6@sha256:e61b455868b416882dc45fe53a5039077de9c932865361fde28d52b31e4a68d2"

	CTLogImage        = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/ct-server-0-6@sha256:17eafff9bc34610d0295654df5adcf6e090bca6581cfc0eb0bb4896405953ac2"
	ClientServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/access-1-0-gamma/client-server-1-0-gamma@sha256:91caede7f666f380bd3e437444386a3818d89d50f28bb468c79294450c6bca9e"
)
