package constants

const (
	TrillianLogSignerImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logsigner@sha256:758951a98791644d2512369019b077c261a685507bbcc119facf34c9d3d378f9"
	TrillianServerImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/logserver@sha256:fe3f936d4261191509b6d14585d569ffa51a0a97448266835fc50e22d929479f"
	TrillianDbImage        = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/database@sha256:f0963d5df411b670c868bde46d49b13d5dc2c146875612b2fd92e7b10cdddcba"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/netcat@sha256:8ae5e563f8bf6bc47f70b09744bff89fa8e26cadbe2d56fabd3565a03a8089ae"

	FulcioServerImage = "quay.io/redhat-user-workloads/rhtas-tenant/fulcio/fulcio-server@sha256:f681ddfdb833bba18f2a2249a80260c9049f523b258bf3924a27796e4716f863"

	RekorRedisImage    = "quay.io/redhat-user-workloads/rhtas-tenant/trillian/redis@sha256:04828065ab7ece69321d851707849e8151c94fc31f7b97e8429a0ae581cdf4b9"
	RekorServerImage   = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/rekor-server@sha256:7db020afd0202ceb22e2f99812650d48b78e48ae63cc57e8221be13ba2005682"
	RekorSearchUiImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor-search-ui/rekor-search-ui@sha256:7f6aa5eb011d5250ccf79375f229034064c5d626bf6f5e4c31beca89dad27093"
	BackfillRedisImage = "quay.io/redhat-user-workloads/rhtas-tenant/rekor/backfill-redis@sha256:aac9c4d61cec2233108235192fe5812ece7a6cd7dcad41dcdcc025faf32d4c06"

	TufImage = "quay.io/redhat-user-workloads/rhtas-tenant/scaffold/tuf-server@sha256:975bc755f80eefa615f796555f09997b4c7e200d19f1b50702ece63d9f942737"

	CTLogImage = "quay.io/redhat-user-workloads/rhtas-tenant/certificate-transparency-go/certificate-transparency-go@sha256:a851350d37cb2105c35c529a30f1b5432c85056e24c95838cfc39b0870187cc6"

	ClientServerImage    = "registry.access.redhat.com/ubi9/httpd-24@sha256:965f7b03ae8f45228bad765ce47bc8956711e854213df0e4cee8623d51317b0a"
	ClientServerImage_cg = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-cg@sha256:5c7e83f13f9ba52c927bcb86abb5f9efa8ca4fe9ab7031709bde7f48829f6d0f"
	ClientServerImage_re = "quay.io/redhat-user-workloads/rhtas-tenant/cli/client-server-re@sha256:d20c57b457650a025c225863462b2f9bf193ca35534dbf2cfcd9f6ab07a091b9"
)
