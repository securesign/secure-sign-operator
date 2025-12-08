package actions

const (
	DbDeploymentName        = "trillian-db"
	DbPvcName               = "trillian-mysql"
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

	// MySQL
	SecretRootPassword = "mysql-root-password"
	SecretPassword     = "mysql-password"
	SecretDatabaseName = "mysql-database"
	SecretUser         = "mysql-user"
	SecretPort         = "mysql-port"
	SecretHost         = "mysql-host"

	// PostgreSQL
	PgSecretPassword     = "postgresql-password"
	PgSecretDatabaseName = "postgresql-database"
	PgSecretUser         = "postgresql-user"
	PgSecretPort         = "postgresql-port"
	PgSecretHost         = "postgresql-host"
)
