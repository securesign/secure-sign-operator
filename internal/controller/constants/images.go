package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:78f4d2603fe4b1fbed8c2484bedc646ddec17feef6ddc3979dc8d19241ce37b2"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:9d942171f0f32ae2317da4236c695f2177f85a0ed94a71f58049440f1583f4e7"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:41f2aa4fad7b1c7175f13ca031ebbd3a09c5ffe06b8c468ae3dd6cc405d36459"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:bdab8117b16ba013a059966084313146458564270ed0a0007dbe6ab9a98638e8"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:7a2783d097d1b8896ea210914d999e7e17bff66fba5fa0386adc46ed17d304c8"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:6c42857864311572667ae2f9697ac812b74e1cba0c1e03ddd6f6f3f06aed7ab6"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:39220599ff9bbcd77ca8188a9c2c2cd75aa914b623bfc5ed01b6d0c607a833b9"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:aed7cb5d3cd161f78fe149474f3b5e1a748580b63845359052f1bbea4a96274e"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:e8bf7e192b260e3f92288ab81f052668f77f0f78bdca961672274d723b55325d"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:c42827f4785c06c0f3e31cf486bbaab5a9d0609131afcd65f60ed63d35d1d8aa"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:649af310d15dc3fc8e07f3a771f9f002f2b62756b8b6d9960760feb9a5a8091b"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:991d67eb32d13970a4ee08920a1369da39d2529c34f1eb092952c827635d8d31"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:b3de68f42f5e2413d69567a5ce3aa2d2e3029253c3b3d0a7f0481843e2b06ddd"
)
