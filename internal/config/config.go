package config

import "time"

var (
	CreateTreeDeadline     int64 = 1200
	Openshift              bool
	OpenshiftAPIServerName string
	APIServerTimeout       time.Duration
)
