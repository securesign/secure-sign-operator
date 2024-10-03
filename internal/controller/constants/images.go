package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:d438373fc3b53b0e318098fc5587e265c0603d7ab3e11fb9819e747a5e2f18a6"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:ccf05883ab992fcf6c3058e6d897c4ce85c13aad54a96499ddeff4a5f056a834"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:ffa1f4f94eb91169c39e5477c0d10ed3020d5fb792651f7464c6465c9d8f558c"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:a1f0c021e9e291394eec61fdd07068466d59a58d61c60e27300a9bae49acb648"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:c89a4891a7054b5cd3774358406ed87eba0ce547dbfd958fb61b0de24140a963"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:45608d663642dce69e3d91101913ba3cc70c2a95d0ce9a23c9116293cd7bc246"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:e01cfb64b011a14a243701a992b7fc3dbc9d7a181bea5cebcfc28593613f7950"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:c454ee91ffc8a1d028d44b999370d98eef751cbfb2b211622d945f51b8afdb84"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:410c5b4f6fd4c50adb8c62a59dadfc9dce9de7a0023b58c9c337f297d7aad0f2"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:053ce379d83675818ddb6f5869bc986d1e210ef8607d081b6b05eb4d4255aa82"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:c7fa18f6dec1fdd308d5a6ed74f5f6bf2bd30d6759d7d2464875b6e80f269fb2"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:d957041e1f10faf087333b9f1d39b2bb4b26edd37a812192e67771c423950def"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:e81f157c97a55cc572bb3a9abcc7d15ccf840b14620ca653b550a0ea9e83da70"
)
