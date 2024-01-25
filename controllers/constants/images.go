package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logsigner-1-5@sha256:04bf51328a940965a4cf3ca7c4b188bf2861c91f639a77b4691733881b82dd35"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logserver-1-5@sha256:56a3a063e5e0729a0bb72eb3f4233b00cb0f4fee22c1df3f01e406b52824ed41"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-database-1-5@sha256:cfb2eb3e5cb2071790c3932991bd6931c2f6eac5626907f928bdc677a70b55e8"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-netcat-1-5@sha256:c876c793f3fb23958e6b381c302b86133ebf2ea49a6153c8a2014ab8a24a4929"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio-1-0-gamma/fulcio-1-0-gamma@sha256:8e80fa6fba6df4cc3065636a7fd926b57327a5ed36f67caf56f162ea7ae0480b"

	RekorRedisImage    = "docker.io/redis@sha256:6c42cce2871e8dc5fb3e843ed5c4e7939d312faf5e53ff0ff4ca955a7e0b2b39"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-server-1-0-gamma@sha256:8772a59796c39f6f8bbe74a628652777e4b187d3c1292ec893797ee87b259497"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-search-ui-1-0-gamma@sha256:1859fcbe036bc48ff4c301b2e8a01fe71f35e5e8a97e6b882e44e6175f34a375"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/tuf-server-0-6@sha256:e61ca335380ccb857cc66ecbb922ac741247956b62abba795fc29da648b91e26"

	CTLogImage        = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/ct-server-0-6@sha256:44c8c0632a3fe797325062bc482018dfb1e44ae592054cba06269cee8356a45e"
	ClientServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/access-1-0-gamma/client-server-1-0-gamma@sha256:60cdd00990d5372889a33cb93258b8dc026a9aa27c6f757bce25a500414d03b6"
)
