package config

import "time"

var (
	CreateTreeDeadline     int64 = 1200
	Openshift              bool
	MonitoringAvailable    bool
	OpenshiftAPIServerName string
	APIServerTimeout       time.Duration
	IngressHostTemplate    = "%[1]s.local"
)
