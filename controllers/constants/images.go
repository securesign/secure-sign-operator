package constants

const (
	TrillianLogSignerImage = "registry.redhat.io/rhtas-tech-preview/trillian-logsigner-rhel9@sha256:015f430f8f966d5cf83cdaad5ba5ca87dd75a83e64abd229c00ddd370f3c393b"
	TrillianServerImage    = "registry.redhat.io/rhtas-tech-preview/trillian-logserver-rhel9@sha256:d6bebd674b5d66bb742bf96dbbea761e2fa73a9fabd9be8a48cefbfc906e990c"
	TrillianDbImage        = "registry.redhat.io/rhtas-tech-preview/trillian-database-rhel9@sha256:3e7210bd4943c266eabaee33a50d86241c66ed69463f208bcc0b0b7373c22af0"
	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/rhtas-tech-preview/trillian-netcat-rhel9@sha256:c4c5a553ea96b2bf98434d9fb21d0a1bf5649b85daa02698748cef89b88c2471"
	FulcioServerImage   = "registry.redhat.io/rhtas-tech-preview/fulcio-rhel9@sha256:dc1f10ddc34cce14a6e35f1448b6d1af1804dfbdb2cfe340ec5d133689a8fa92"
	RekorServerImage    = "registry.redhat.io/rhtas-tech-preview/rekor-server-rhel9@sha256:45a5dfcedb1aaa7c15f4f444a9d0440ea3413e8a245a77a5aa562205a91627a3"
	RekorSearchUiImage  = "registry.redhat.io/rhtas-tech-preview/rekor-search-ui-rhel9@sha256:325f1e84936c31e02bddb2bc4fff07c3a55c2e556deba72e99f4ec99aa589cca"
	RekorRedisImage     = "registry.redhat.io/rhtas-tech-preview/redis-trillian-rhel9@sha256:914089e0e8407420ce07f44dad49da75d34c88eac6314ea8d6e45ff0745e4b42"

	TufImage          = "registry.redhat.io/rhtas-tech-preview/tuf-server-rhel9@sha256:0702c1d59306e743a8fcf01785c25c5d5edb64199c8e626b4438ffb08b88a5e5"
	CTLogImage        = "registry.redhat.io/rhtas-tech-preview/ct-server-rhel9@sha256:c99e09403fef657f9e03d29991e042d9b26d4a3ebd5a74c50bb7b2b6555693ca"
	ClientServerImage = "registry.redhat.io/rhtas-tech-preview/client-server-rhel9@sha256:eebda321cdc0cb5bd0ce2df83a661e566f5a48a34bd9d192e72d4300347956e3"
)
