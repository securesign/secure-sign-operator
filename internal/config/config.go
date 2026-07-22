package config

import "time"

var (
	CreateTreeDeadline       int64 = 1200
	Openshift                bool
	OpenshiftAPIServerName   string
	APIServerTimeout         time.Duration
	IngressHostTemplate      = "%[1]s.local"
	DisableClusterTLSProfile bool
)
