package actions

const (
	DbDeploymentName        = "trillian-db"
	DbPvcName               = "trillian-mysql"
	LogserverDeploymentName = "trillian-logserver"
	LogsignerDeploymentName = "trillian-logsigner"

	DbComponentName        = "trillian-db"
	LogServerComponentName = "trillian-logserver"
	LogSignerComponentName = "trillian-logsigner"

	RBACName = "trillian"

	DbCondition     = "DBAvailable"
	ServerCondition = "LogServerAvailable"
	SignerCondition = "LogSignerAvailable"
)
