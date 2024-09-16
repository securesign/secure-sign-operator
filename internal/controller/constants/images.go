package constants

var (
	TrillianLogSignerImage = "quay.io/securesign/trillian-logsigner@sha256:3a73910e112cb7b8ad04c4063e3840fb70f97ed07fc3eb907573a46b2f8f6b7b"
	TrillianServerImage    = "quay.io/securesign/trillian-logserver@sha256:23579db8db307a14cad37f5cb1bdf759611decd72d875241184549e31353387f"
	TrillianDbImage        = "quay.io/securesign/trillian-database@sha256:310ecbd9247a2af587dd6bca1b262cf5d753938409fb74c59a53622e22eb1c31"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "quay.io/securesign/fulcio-server@sha256:a384c19951fb77813cdefb8057bbe3670ef489eb61172d8fd2dde47b23aecebc"

	RekorRedisImage    = "quay.io/securesign/trillian-redis@sha256:c936589847e5658e3be01bf7251da6372712bf98f4d100024a18ea59cfec5975"
	RekorServerImage   = "quay.io/securesign/rekor-server@sha256:96efc463b5f5fa631cca2e1a2195bb0abbd72da0c5083a9d90371d245d01387d"
	RekorSearchUiImage = "quay.io/securesign/rekor-search-ui@sha256:8ed9d49539e2305c2c41e2ad6b9f5763a53e93ab7590de1c413d846544091009"
	BackfillRedisImage = "quay.io/securesign/rekor-backfill-redis@sha256:22016378cf4a312ac7b15067e560ea42805c168ddf2ae64adb2fcc784bb9ba15"

	TufImage = "quay.io/securesign/tuffer@sha256:fc0160028b0bcbc03c69156584ead3dfec6d517dab305386ee238cc0e87433de"

	CTLogImage = "quay.io/securesign/certificate-transparency-go@sha256:671c5ea4de7184f0dcdd6c6583d74dc8b0b039799c57efb5e8a31981cd9b415e"

	HttpServerImage         = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg    = "quay.io/securesign/cli-client-server-cg@sha256:0469bef1617c60481beda30947f279a0b106d0e54c600e823064a2b5b89bc120"
	ClientServerImage_re    = "quay.io/securesign/client-server-re@sha256:7990157e558dc5ff6e315c84a107bbadc7aeb3aaed39a9171e751671be5d89f0"
	ClientServerImage_f     = "quay.io/securesign/client-server-f@sha256:aca918e6994ad5f95c71f725428fc3f2865299b1860c2740d1c18f03324cc3c9"
	SegmentBackupImage      = "quay.io/securesign/segment-backup-job@sha256:625b5beef8b97d0e9fdf1d92bacd31a51de6b8c172e9aac2c98167253738bb61"
	TimestampAuthorityImage = "quay.io/securesign/timestamp-authority@sha256:788f298596b5c0c70e06ac210f8e68ce7bf3348c56b7f36eb6b84cdd85f0d01d"
)
