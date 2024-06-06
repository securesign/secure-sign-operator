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

	RBACName = "trillian"

	DbCondition     = "DBAvailable"
	ServerCondition = "LogServerAvailable"
	SignerCondition = "LogSignerAvailable"
)
