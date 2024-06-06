package actions

const (
	ServerDeploymentName     = "rekor-server"
	RedisDeploymentName      = "rekor-redis"
	SearchUiDeploymentName   = "rekor-search-ui"
	RBACName                 = "rekor"
	MonitoringRoleName       = "prometheus-k8s-rekor"
	ServerComponentName      = "rekor-server"
	RedisComponentName       = "rekor-redis"
	UIComponentName          = "rekor-ui"
	BackfillRedisCronJobName = "backfill-redis"
	UICondition              = "UiAvailable"
	ServerCondition          = "ServerAvailable"
	RedisCondition           = "RedisAvailable"
	SignerCondition          = "SignerAvailable"
)
