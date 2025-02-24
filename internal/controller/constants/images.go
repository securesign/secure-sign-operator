package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:0a9713466a55b5c79eae8dce71fd33184fdd07e59ffd530ca17b60895374902c"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:89eec6d832ff3cd032ace453f950c88b075994e5b905d5347fd927202876c512"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:614ba2e79a5f5230bea925d001756c66ac09deac24aa529628095540d8219180"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:f58588d336b578da548831d555d627614eabf993a693f570047c2a2bafff5b1b"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:0b708607468e175139de6838b90b7fa2fb22985e2f0e8caa81e0f97b0a1a590c"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:993394a07f178f89eb103b33fbf7bc007db3cca98eaa79e01b6e6a1ba2a302e6"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:1432b47ddd881eb1909453e939c791825e7853b4abc00a12dddd948f99022ab3"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:59b06a2fc7290b0dd7738f09c0d3fe19eab69f2bea10c998c481da3139c25c78"

	TufImage = "registry.redhat.io/rhtas/tuffer-rhel9@sha256:c327853589a048b0848773bc19e74edc598498eda2021914e92b4fcfd1059a02"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:5e85b4eb162c1d136add09491af72413a5d7475df041ce4340b919302bee6600"

	HttpServerImage = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"

	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:53a7929f6426f460e5386c505957b0de0b4a92c6f6bad8dd678851e2867f42af"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:0fdd5e119325e8c30f5ef0da9b0a78469143a3d222e8b92d0d972acbed8db99c"
	ClientServerImage       = "registry.redhat.io/rhtas/client-server-rhel9@sha256:ce30450e9e3aee7368bd9ba7f756d7af0f7c0e052cd57951256adaa9c78fb562"
)
