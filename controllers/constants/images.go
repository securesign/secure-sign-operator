package constants

const (
	TrillianLogSignerImage = "registry.redhat.io/rhtas-tech-preview/trillian-logsigner-rhel9@sha256:fa2717c1d54400ca74cc3e9038bdf332fa834c0f5bc3215139c2d0e3579fc292"
	TrillianServerImage    = "registry.redhat.io/rhtas-tech-preview/trillian-logserver-rhel9@sha256:43bfc6b7b8ed902592f19b830103d9030b59862f959c97c376cededba2ac3a03"
	TrillianDbImage        = "registry.redhat.io/rhtas-tech-preview/trillian-database-rhel9@sha256:fe4758ff57a9a6943a4655b21af63fb579384dc51838af85d0089c04290b4957"

	// TODO: remove and check the DB pod status
	TrillianNetcatImage = "registry.redhat.io/rhtas-tech-preview/trillian-netcat-rhel9@sha256:b9fa895af8967cceb7a05ed7c9f2b80df047682ed11c87249ca2edba86492f6e"

	FulcioServerImage = "registry.redhat.io/rhtas-tech-preview/fulcio-rhel9@sha256:0421d44d2da8dd87f05118293787d95686e72c65c0f56dfb9461a61e259b8edc"

	RekorRedisImage  = "docker.io/redis@sha256:6c42cce2871e8dc5fb3e843ed5c4e7939d312faf5e53ff0ff4ca955a7e0b2b39"
	RekorServerImage = "registry.redhat.io/rhtas-tech-preview/rekor-server-rhel9@sha256:8ee7d5dd2fa1c955d64ab83d716d482a3feda8e029b861241b5b5dfc6f1b258e"

	TufImage = "registry.redhat.io/rhtas-tech-preview/tuf-server-rhel9@sha256:413e361de99f09e617084438b2fc3c9c477f4a8e2cd65bd5f48271e66d57a9d9"

	CTLogImage = "registry.redhat.io/rhtas-tech-preview/ct-server-rhel9@sha256:6124a531097c91bf8c872393a6f313c035ca03eca316becd3c350930d978929f"
)
