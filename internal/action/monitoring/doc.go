// Package monitoring provides a generic action for managing Prometheus
// ServiceMonitor resources across operator components.
//
// The action creates or deletes a ServiceMonitor based on the component's
// monitoring configuration:
//
//   - Enabled: creates or updates the ServiceMonitor with correct selector
//     labels and endpoint configuration (HTTP or HTTPS).
//   - Disabled: deletes the ServiceMonitor if it exists.
//   - CRD missing: returns a retriable error on create, silently ignores on delete.
//
// Each component implements the [Config] interface with two methods:
//
//   - IsEnabled: whether ServiceMonitor creation is active.
//   - TLS: returns the TLS struct; CertRef != nil enables HTTPS, zero value uses HTTP.
//
// Usage:
//
//	func NewCreateMonitorAction() action.Action[*rhtasv1.Fulcio] {
//	    return monitoring.NewAction(
//	        ComponentName, MonitoringRoleName, DeploymentName,
//	        "", // conditionType: empty = no status condition on error
//	        fulcioMonitoringConfig{},
//	    )
//	}
package monitoring
