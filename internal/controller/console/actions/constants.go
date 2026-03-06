package actions

const (
	DbDeploymentName  = "console-db"
	DbPvcName         = "console-db"
	UIDeploymentName  = "console-ui"
	ApiDeploymentName = "console-api"

	DbComponentName   = "console-db"
	UIComponentName   = "console-ui"
	ApiComponentName  = "console-api"
	ApiMonitoringName = "prometheus-k8s-api"

	UITLSSecret       = "%s-console-ui-tls"
	ApiTLSSecret      = "%s-console-api-tls"
	DatabaseTLSSecret = "%s-console-db-tls"

	RBACUIName  = "console-ui"
	RBACApiName = "console-api"
	RBACDbName  = "console-db"

	DbCondition  = "DBAvailable"
	UICondition  = "UIAvailable"
	ApiCondition = "ApiAvailable"

	UiServerPort      = 8080
	UiServerPortName  = "http"
	UiMetricsPort     = 8090
	UiMetricsPortName = "metrics"

	ApiServerPort      = 8080
	ApiServerPortName  = "http"
	ApiMetricsPort     = 8090
	ApiMetricsPortName = "metrics"

	SecretRootPassword = "mysql-root-password"
	SecretPassword     = "mysql-password"
	SecretDatabaseName = "mysql-database"
	SecretUser         = "mysql-user"
	SecretPort         = "mysql-port"
	SecretHost         = "mysql-host"
	SecretDsn          = "dsn"
)
