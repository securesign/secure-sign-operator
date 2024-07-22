package actions

const (
	ServerDeploymentName       = "rekor-server"
	ServerDeploymentPortName   = "http"
	ServerDeploymentPort       = 80
	ServerTargetDeploymentPort = 3000
	MetricsPortName            = "metrics"
	MetricsPort                = 2112
	RedisDeploymentName        = "rekor-redis"
	RedisDeploymentPortName    = "resp"
	RedisDeploymentPort        = 6379
	SearchUiDeploymentName     = "rekor-search-ui"
	SearchUiDeploymentPortName = "http"
	SearchUiDeploymentPort     = 3000
	RBACName                   = "rekor"
	MonitoringRoleName         = "prometheus-k8s-rekor"
	ServerComponentName        = "rekor-server"
	RedisComponentName         = "rekor-redis"
	UIComponentName            = "rekor-ui"
	BackfillRedisCronJobName   = "backfill-redis"
	UICondition                = "UiAvailable"
	ServerCondition            = "ServerAvailable"
	RedisCondition             = "RedisAvailable"
	SignerCondition            = "SignerAvailable"
)
