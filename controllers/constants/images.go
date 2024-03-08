package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logsigner@sha256:2136e6a0e55359dfcbc38dee86f0769fa2c27cf1235c7ea953406f26075dd12a"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logserver@sha256:fe08b91cfac78a5e441ba889980e207425216fb2fa1235d16876582a2a7278ac"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/database@sha256:ce37d05a70d742ea12dac1d6df46f70a22d11bd71a00d2f1cd1479633ad91f63"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio/fulcio-server@sha256:afc0b39f25fc2a503e60b67336133ba38a9fed5043814b0bc78df938059d7f98"

	RekorRedisImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/redis@sha256:d68018e6a882f2445ff1efe26bddf1fdb25082b3bdb2552a7b7d7ab572bef01e"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/rekor-server@sha256:6982ff3da63f85fe64b3eed454f4388566eea8751b2f5829749591a659b2c155"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-search-ui/rekor-search-ui@sha256:64470c5f026b539f8e8f419ccd37043d28520251a724914f4af17ce658829fd7"
	BackfillRedisImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/backfill-redis@sha256:e118d4ca4fdcecac7e22fca203a64020d28db47d053d1880b8c578bc65859ac0"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold/tuf-server@sha256:d494df01d273b42b8c829f16d2f1556080c82f4b5d18e13e77c2caa00dfea9e1"

	CTLogImage = "quay.io/redhat-user-workloads/rhtas-tenant/certificate-transparency-go/certificate-transparency-go@sha256:74a07a36dd715e23433871ae079f9f4e6b66037af4068cab102e29f8f20f0b69"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-cg@sha256:4d75da6f9f3ab1963cf8fa00eaf331b7704289cf32d91c25ffd0092d2dd3a299"
	ClientServerImage_re = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-re@sha256:1484b8af233bcdc26c9773f3868e2d6a38722ae38b4fd3baf7430e3a7213b15d"
)
