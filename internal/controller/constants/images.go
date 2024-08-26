package constants

var (
	TrillianLogSignerImage = "registry.redhat.io/rhtas/trillian-logsigner-rhel9@sha256:37028258a88bba4dfaadb59fc88b6efe9c119a808e212ad5214d65072abb29d0"
	TrillianServerImage    = "registry.redhat.io/rhtas/trillian-logserver-rhel9@sha256:994a860e569f2200211b01f9919de11d14b86c669230184c4997f3d875c79208"
	TrillianDbImage        = "registry.redhat.io/rhtas/trillian-database-rhel9@sha256:909f584804245f8a9e05ecc4d6874c26d56c0d742ba793c1a4357a14f5e67eb0"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "registry.redhat.io/rhtas/fulcio-rhel9@sha256:67495de82e2fcd2ab4ad0e53442884c392da1aa3f5dd56d9488a1ed5df97f513"

	RekorRedisImage    = "registry.redhat.io/rhtas/trillian-redis-rhel9@sha256:01736bdd96acbc646334a1109409862210e5273394c35fb244f21a143af9f83e"
	RekorServerImage   = "registry.redhat.io/rhtas/rekor-server-rhel9@sha256:133ee0153e12e6562cfea1a74914ebdd7ee76ae131ec7ca0c3e674c2848150ae"
	RekorSearchUiImage = "registry.redhat.io/rhtas/rekor-search-ui-rhel9@sha256:8c478fc6122377c6c9df0fddf0ae42b6f6b1648e3c6cf96a0558f366e7921b2b"
	BackfillRedisImage = "registry.redhat.io/rhtas/rekor-backfill-redis-rhel9@sha256:88869eb582cbb94baa50c212689c50ed405cc94669c2c03f781b12ad867827ce"

	TufImage = "registry.redhat.io/rhtas/tuf-server-rhel9@sha256:092ee1327639c2c8fee809ea66ecd11ca7bc9951c1832391df0df6f1f4d62a6a"

	CTLogImage = "registry.redhat.io/rhtas/certificate-transparency-rhel9@sha256:a0c7d71fc8f4cb7530169a6b54dc3a67215c4058a45f84b87bb04fc62e6e8141"

	ClientServerImage       = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg    = "registry.redhat.io/rhtas/client-server-cg-rhel9@sha256:987c630213065a6339b2b2582138f7b921473b86dfe82e91a002f08386a899ed"
	ClientServerImage_re    = "registry.redhat.io/rhtas/client-server-re-rhel9@sha256:dc4667af49ce6cc70d70bf83cab9d7a14b424d8ae1aae7e4863ff5c4ac769a96"
	ClientServerImage_f     = "registry.redhat.io/rhtas/client-server-f-rhel9@sha256:65fb59c8f631215d9752fc4f41571eb2750ecaaa8555083f58baa6982e97d192"
	SegmentBackupImage      = "registry.redhat.io/rhtas/segment-reporting-rhel9@sha256:e9fc117dc7cc089aa765ed92f40fcaaa1220688ace57e5a5c909917be641a75d"
	TimestampAuthorityImage = "registry.redhat.io/rhtas/timestamp-authority-rhel9@sha256:3fba2f8cd09548d2bd2dfff938529952999cb28ff5b7ea42c1c5e722b8eb827f"
)
