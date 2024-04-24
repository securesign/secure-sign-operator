package constants

const (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:aa7812254a582c12bf71834f1617a81f76d00a5ee90413d7d707ed5068e21220"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:61320904f81c5d389c052222311a1357504a14b0de00426013cb8f29690220bf"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:4989f92e18a541f12fb6174bd6686a24362a24700813e352c8c2c8eff4ab943f"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:f333772eb0cd23360516da4a7a50813a59d67c690c2b6baef4bc4b6094d1116b"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:8c12d98f96ed3daa3ea7ce03d52e8b6748c4b7ab20eee98c1d1ad031d14af00e"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:133e42c7dd44ce6d12a3cee28ead1a4eb1a99ebaaa07d67d4362eec7561ae672"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:0c8a24911435b3d8b9c137e8105192a3f513d775cc2728a87506c68785975f7b"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:46bf598e4959fd032f53ad7728ba9dc23f463b8b841d207d31def4f0d4990888"

	TufImage = "registry.redhat.io/rhtas/tuf-server-rhel9@sha256:6e07be763052f724ac96739afed4b35509b95d88e3d5d86748c5b931f94639aa"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:1419a048cb5095b3f65d08224e6f94c6eb166d8d5a16707942aed2880992ddee"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:ebfa423249443514c019c930b125011ed64b3eea5910d6ad9c52cc4a3158d992"
	ClientServerImage_re = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:81e6f02989aca33033132488c77525840c4fc47fa49929e4107b8fe176eb52a9"
	SegmentBackupImage   = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:e5a733c8040ca357dd99931e18fec0c3aa1cfacd7391ef57b9752d71a6a1e190"
)
