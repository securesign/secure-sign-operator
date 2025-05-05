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
	MonitorDeploymentName      = "rekor-monitor"
	SearchUiDeploymentName     = "rekor-search-ui"
	SearchUiDeploymentPortName = "http"
	SearchUiDeploymentPort     = 3000

	RedisTlsSecret = "%s-rekor-redis-tls"

	RBACName         = "rekor"
	RBACUIName       = "rekor-ui"
	RBACRedisName    = "rekor-redis"
	RBACBackfillName = "rekor-backfill"

	MonitoringRoleName       = "prometheus-k8s-rekor"
	ServerComponentName      = "rekor-server"
	RedisComponentName       = "rekor-redis"
	MonitorComponentName     = "rekor-monitor"
	UIComponentName          = "rekor-ui"
	BackfillRedisCronJobName = "backfill-redis"
	UICondition              = "UiAvailable"
	ServerCondition          = "ServerAvailable"
	RedisCondition           = "RedisAvailable"
	MonitorCondition         = "MonitorAvailable"
	SignerCondition          = "SignerAvailable"
)
