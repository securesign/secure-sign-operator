package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:37028258a88bba4dfaadb59fc88b6efe9c119a808e212ad5214d65072abb29d0"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:994a860e569f2200211b01f9919de11d14b86c669230184c4997f3d875c79208"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:909f584804245f8a9e05ecc4d6874c26d56c0d742ba793c1a4357a14f5e67eb0"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:1ebf0160eadd707bd087d46e08272cab5ec2935446c3ddb6b9a2da24ad5c84bf"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:f9446fddb68b7b0f8a260e9491d824ccb6eff1f2fb83696383d90f593179726a"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:73767ee1df9c9e63d5d20f5eee83dcb828fea4c5400ee771114577c02f335f66"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:8486888c1af02c4d6bade9d00e2c84589b206e2e195f266c03ed1937babcdaa7"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:fd88470986177361fa58b5283ed807c19638c30be9f1391252f727db768543f8"

	TufImage = "registry.redhat.io/rhtas/tuf-server-rhel9@sha256:3c9f0639534786f05a5b2a12ebadeb3c9c13fa66785e0e0470fe271406fe6a7c"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:1588b8e97456806b4f76f8e1e63f3c089f2bc3b898f18f43c22d03dbc03b4c15"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:105abb4bc44fc15b62df386be45d75bea40429aee5d2cbb3d599b8614e1f7b13"
	ClientServerImage_re = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:c39bec557d519aaaedf09dfa6c54587914c6a253ed3c121bcbb4ae1827d1ec2d"
	SegmentBackupImage   = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:5b7006f686f9b0c7cbed8ee5b599c72b952fd0f02d0388aa90c2b420bb7a78ba"
)
