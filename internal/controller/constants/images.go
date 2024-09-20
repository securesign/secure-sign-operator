package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:b83d806de7d9653d4ce4cf2c1db7b5f8aa607f3888a99c4924477b5cfb48c930"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:3d017de5adf2ab58f5a87dcad5ccd38a2a40003834ef09d3bc17d8946387fa05"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:2f20f41d6646d3b3fe816663491a4fa86b362d1c42b8bd1968a6be301eeb11fe"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:02dc2af135b4eaa16deec597187fc9c4eb1d7e395631d0566df80eb3e0aaa84e"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:16ad1b2a0fc40792e26b3b84868315064469250b24321d5ffb7980c0e7b029da"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:7f7ede4c0a51d3f8c459cc86bcd33c8858992764f910d4c882c55bf7bcbeb91f"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:9973ce7c5ceed4a6f5b45c69a22ba98f5c6ad324e212ba882415b85488528fc1"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:02d409438a038007f831abaf9eac3cd86f203fbb6b6dece4d0f70dc1b52cd79c"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:1beb250dfc24a0c094799afda075989cf6f7eb1212d655571fc9054f74961f89"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:6fef78c77e6b2a926d7535d46d86e7bbda3e30ad9d5d6653bcb96698b56594fc"

	HttpServerImage         = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg    = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:f0669481c6483c6025d925ec9ad64199acc44cee2aaf7ea6aab64e2bd5d85485"
	ClientServerImage_re    = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:bde7470ea648ffd77fda2ea362858115b8086b92ffb8c2e3a74107f955f7c644"
	ClientServerImage_f     = "registry.redhat.io/rhtas/client-server-f-rhel9@sha256:8c8c4bfcbc8728ee46a427a4179622e4437e3502aa4b29af7539bf2eee999ff6"
	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:c7fa18f6dec1fdd308d5a6ed74f5f6bf2bd30d6759d7d2464875b6e80f269fb2"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:d957041e1f10faf087333b9f1d39b2bb4b26edd37a812192e67771c423950def"
)
