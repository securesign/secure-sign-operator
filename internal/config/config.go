package config

var (
	CreateTreeDeadline int64 = 1200
	Openshift          bool

	IngressHostTemplate = "%[1]s.%[2]s.traefik.me"
)
