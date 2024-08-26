package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:3a73910e112cb7b8ad04c4063e3840fb70f97ed07fc3eb907573a46b2f8f6b7b"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:23579db8db307a14cad37f5cb1bdf759611decd72d875241184549e31353387f"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:310ecbd9247a2af587dd6bca1b262cf5d753938409fb74c59a53622e22eb1c31"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:00f7a99625ecb3b71382d5b552cac66d0228d5a0229cbe7e230e879156c406ab"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:c936589847e5658e3be01bf7251da6372712bf98f4d100024a18ea59cfec5975"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:91db87e360a0559c12ecd724ff34841563182f1bae826e3781232daed9a2034f"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:8ed9d49539e2305c2c41e2ad6b9f5763a53e93ab7590de1c413d846544091009"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:d0f42e2342976c897e5649bc6299a593c9910d52d615ca004ed4a14d42f670f3"

	TufImage = "registry.redhat.io/rhtas/tuf-server-rhel9@sha256:ff5bb03c4001f207f1dcc24490fc9fedcdae77c05e0095cf1030ef79b77347e1"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:671c5ea4de7184f0dcdd6c6583d74dc8b0b039799c57efb5e8a31981cd9b415e"

	ClientServerImage       = "registry.access.redhat.com/ubi9/httpd-24@sha256:ac6c0b1f3d3f4e412dffd72b4223d707c7d50e674a493f1ebd46f99c1a08b0c7"
	ClientServerImage_cg    = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:0469bef1617c60481beda30947f279a0b106d0e54c600e823064a2b5b89bc120"
	ClientServerImage_re    = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:7990157e558dc5ff6e315c84a107bbadc7aeb3aaed39a9171e751671be5d89f0"
	ClientServerImage_f     = "registry.redhat.io/rhtas/client-server-f-rhel9@sha256:aca918e6994ad5f95c71f725428fc3f2865299b1860c2740d1c18f03324cc3c9"
	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:e9fc117dc7cc089aa765ed92f40fcaaa1220688ace57e5a5c909917be641a75d"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:788f298596b5c0c70e06ac210f8e68ce7bf3348c56b7f36eb6b84cdd85f0d01d"
)
