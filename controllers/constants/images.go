package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logsigner@sha256:0f55a1065bdeca25bee583c4b3666795a749d43b6a490b0f77c5b9913d55bb2d"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logserver@sha256:bd457ba83dddf9c5a278e9c18ddf21f5ba11834590635fc197c25a4f98dc9afe"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/database@sha256:995a05b679ac0953514f3744fa8b19f24bedccfadf5a32813e678cea175d3e88"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio/fulcio-server@sha256:14a8cb8baf1868d3c82ff3c0037579599007b3bee6ac21906420143d55ac5561"

	RekorRedisImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/redis@sha256:a39b745eb2878191d82ff002b61e4fb0a4004a416751d5fd62eabc72e8b81647"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/rekor-server@sha256:e4a5dd78a96686ba66b5723dc3516a2f2b717162aabff42a969dece606ca43c9"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-search-ui/rekor-search-ui@sha256:6ba83b2e09d77c0e3cc21739cb51c6639a9a8586de9b8e9924983795dad4f9ba"
	BackfillRedisImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/backfill-redis@sha256:0097a4525aa962a14ac1aaef4175f5e99be557793c93dfd68790f0e233d72ede"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold/tuf-server@sha256:fe1fb5ee68635a05c831ac5f596d94869b48d2e3756bc0f4094333de7ca56833"

	CTLogImage = "quay.io/redhat-user-workloads/rhtas-tenant/certificate-transparency-go/certificate-transparency-go@sha256:31227e32767658664dad905547dacbcfc8f634d7d21a43787868a8bd8905c986"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-cg@sha256:18deade47e3f1be1179bba021270edba0560f7546a4d0273179c5901104a3ffc"
	ClientServerImage_re = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-re@sha256:fc956d235060f9ce8e97410043bb80dd8c79ab43c220d2bbf46b0aec27ff7d19"
)
