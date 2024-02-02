package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logsigner-1-5@sha256:015f430f8f966d5cf83cdaad5ba5ca87dd75a83e64abd229c00ddd370f3c393b"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-logserver-1-5@sha256:d6bebd674b5d66bb742bf96dbbea761e2fa73a9fabd9be8a48cefbfc906e990c"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-database-1-5@sha256:3e7210bd4943c266eabaee33a50d86241c66ed69463f208bcc0b0b7373c22af0"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/trillian-netcat-1-5@sha256:c4c5a553ea96b2bf98434d9fb21d0a1bf5649b85daa02698748cef89b88c2471 "

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio-1-0-gamma/fulcio-1-0-gamma@sha256:dc1f10ddc34cce14a6e35f1448b6d1af1804dfbdb2cfe340ec5d133689a8fa92"

	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-server-1-0-gamma@sha256:45a5dfcedb1aaa7c15f4f444a9d0440ea3413e8a245a77a5aa562205a91627a3"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-1-0-gamma/rekor-search-ui-1-0-gamma@sha256:325f1e84936c31e02bddb2bc4fff07c3a55c2e556deba72e99f4ec99aa589cca"
	RekorRedisImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian-1-0-gamma/redis-0-6@sha256:914089e0e8407420ce07f44dad49da75d34c88eac6314ea8d6e45ff0745e4b42"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/tuf-server-0-6@sha256:0702c1d59306e743a8fcf01785c25c5d5edb64199c8e626b4438ffb08b88a5e5"

	CTLogImage        = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold-1-0-gamma/ct-server-0-6@sha256:c99e09403fef657f9e03d29991e042d9b26d4a3ebd5a74c50bb7b2b6555693ca"
	ClientServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/access-1-0-gamma/client-server-1-0-gamma@sha256:eebda321cdc0cb5bd0ce2df83a661e566f5a48a34bd9d192e72d4300347956e3"
)
