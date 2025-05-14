package actions

const (
	ServerDeploymentName            = "rekor-server"
	ServerDeploymentPortName        = "http"
	ServerDeploymentPort            = 80
	ServerTargetDeploymentPort      = 3000
	MetricsPortName                 = "metrics"
	MetricsPort                     = 2112
	RedisDeploymentName             = "rekor-redis"
	RedisDeploymentPortName         = "resp"
	RedisDeploymentPort             = 6379
	MonitorDeploymentName           = "rekor-monitor"
	OtelCollectorDeploymentName     = "otel-collector"
	OtelCollectorGrpcPortName       = "grpc"
	OtelCollectorGrpcPort           = 4317
	OtelCollectorPrometheusPortName = "prometheus"
	OtelCollectorPrometheusPort     = 9464
	SearchUiDeploymentName          = "rekor-search-ui"
	SearchUiDeploymentPortName      = "http"
	SearchUiDeploymentPort          = 3000
	RBACName                        = "rekor"
	MonitoringRoleName              = "prometheus-k8s-rekor"
	ServerComponentName             = "rekor-server"
	RedisComponentName              = "rekor-redis"
	MonitorComponentName            = "rekor-monitor"
	OtelCollectorComponentName      = "otel-collector"
	UIComponentName                 = "rekor-ui"
	BackfillRedisCronJobName        = "backfill-redis"
	UICondition                     = "UiAvailable"
	ServerCondition                 = "ServerAvailable"
	RedisCondition                  = "RedisAvailable"
	MonitorCondition                = "MonitorAvailable"
	OtelCollectorCondition          = "OtelCollectorAvailable"
	SignerCondition                 = "SignerAvailable"
)
