package actions

const (
	DbDeploymentName        = "trillian-db"
	DbPvcName               = "trillian-db"
	LogserverDeploymentName = "trillian-logserver"
	LogsignerDeploymentName = "trillian-logsigner"

	DbComponentName         = "trillian-db"
	LogServerComponentName  = "trillian-logserver"
	LogServerMonitoringName = "prometheus-k8s-logserver"
	LogSignerComponentName  = "trillian-logsigner"
	LogSignerMonitoringName = "prometheus-k8s-logsigner"

	LogServerTLSSecret = "%s-trillian-logserver-tls"
	LogSignerTLSSecret = "%s-trillian-logsigner-tls"
	DatabaseTLSSecret  = "%s-trillian-db-tls"

	RBACServerName = "trillian-logserver"
	RBACSignerName = "trillian-logsigner"
	RBACDbName     = "trillian-db"

	DbCondition     = "DBAvailable"
	ServerCondition = "LogServerAvailable"
	SignerCondition = "LogSignerAvailable"

	ServerPort      = 8091
	ServerPortName  = "grpc"
	MetricsPort     = 8090
	MetricsPortName = "metrics"

	SecretRootPassword = "db-root-password" // Only used for MySQL; ignored for PostgreSQL
	SecretPassword     = "db-password"
	SecretDatabaseName = "db-name"
	SecretUser         = "db-user"
	SecretPort         = "db-port"
	SecretHost         = "db-host"
)
