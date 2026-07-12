package actions

const (
	ApiDeploymentName = "console-api"
	UIDeploymentName  = "console-ui"

	ApiComponentName = "console-api"
	UIComponentName  = "console-ui"

	RBACApiName = "console-api"
	RBACUIName  = "console-ui"

	ApiCondition = "ApiAvailable"
	UICondition  = "UIAvailable"

	ApiTLSSecret = "%s-console-api-tls"

	ApiPort     int32 = 8080
	ApiPortName       = "http"

	UIPort     int32 = 8080
	UIPortName       = "http"
)
