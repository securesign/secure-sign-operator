package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:758bfd5455d0ca96f5404229a73fdcd56686564fc4c315ca3468ebe8588dd9ca"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:1673f08529a94671f881edec6fd29b73f43c2115dc86b643f9dd79d7e36be38e"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:58140d6b7fee762b148336b342a6745b1910ab758a701b2e8077b1fca2af1180"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:e16accb01b2ca6bf4ebd60c6ce7faab73b784828578ee74305a3a97ff9db1e26"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:06382df99dfff9f002365a31eee0855f8032e351bc9583e42e489ab7623fe287"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:d4ea970447f3b4c18c309d2f0090a5d02260dd5257a0d41f87fefc4f014a9526"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:5eabf561c0549d81862e521ddc1f0ab91a3f2c9d99dcd83ab5a2cf648a95dd19"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:0ad1961d6dd1c8636f5f1bcb5ffdbea51ad29b88c6539df366305ef801c79877"

	TufImage = "registry.redhat.io/rhtas/tuf-server-rhel9@sha256:6c26d2394b1d82995a6f3d3483d2c18befd6a4ae826af2cd25aac30dfabef1cc"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:0765d248fd1c4b4f8cbcbee23f35506c08c9eeb96a8b0e8ff1511319be6c0ae6"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:f0c8f5129c0f611332f7b34dfc5f0dc39c73ed8a985a0845d1fe6ce31d2ba020"
	ClientServerImage_re = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:83b9f9a5ea40473c8525a16d9965b1ed1eb2d8f7c2ec1d17ae38e5f10ac1652e"
	SegmentBackupImage   = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:ea37c68775b14e7d590b6eeec3c9b59eba8e988860fc04d40775da3957d6d040"
)
