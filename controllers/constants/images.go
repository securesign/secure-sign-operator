package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logsigner@sha256:f8199e0b14f391574181a3e38659a2ec5baeb65ba5101ac63b5b9785ae01c214"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logserver@sha256:cdb2fa8ef85a9727c2b306652e4127ee4b2723cd361a04f364f4a96d60194777"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/database@sha256:145560da2f030ab6574d62c912b757d5537e75e4ec10e0d26cf56a67b1573969"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/openshift4/ose-tools-rhel8@sha256:486b4d2dd0d10c5ef0212714c94334e04fe8a3d36cf619881986201a50f123c7"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio/fulcio-server@sha256:f333772eb0cd23360516da4a7a50813a59d67c690c2b6baef4bc4b6094d1116b"

	RekorRedisImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/redis@sha256:0804a6634b8836cb2e957ee16d54e8d6ab94d311362a48baf238b1f575d79934"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/rekor-server@sha256:a2075576589bec3c4544db4368732cb1388e8f5a3cb2a739d943cee601e64b74"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-search-ui/rekor-search-ui@sha256:1a8c0448c294e33a3ec6d11f45e886d3ad606b221ed81d114f6d91257a968209"
	BackfillRedisImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/backfill-redis@sha256:27a016efec4dca3f029d0b7ac0fc02cbac7bd44051ebd0e2f458f8b5b9fc8972"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold/tuf-server@sha256:b6875d661113f34911075264950a4a507090dddb2c01c313885ea367c113ec08"

	CTLogImage = "quay.io/redhat-user-workloads/rhtas-tenant/certificate-transparency-go/certificate-transparency-go@sha256:1419a048cb5095b3f65d08224e6f94c6eb166d8d5a16707942aed2880992ddee"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:7874b82335a80269dcf99e5983c2330876f5fe8bdc33dc6aa4374958a2ffaaee"
	ClientServerImage_cg = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-cg@sha256:eb49058e7b12a2f48e363c09ef438aeb8500e83a4fb91451197f6bc2fe2e61de"
	ClientServerImage_re = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-re@sha256:dd05e0575f1ee94ffe8a299a3baf5af1a5b77299f4f356daf7826d6e4814cd3d"
)
