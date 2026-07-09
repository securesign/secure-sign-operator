package monitoring

import "errors"

var (
	// ErrServiceMonitorCRDMissing is returned when ServiceMonitor creation is
	// requested but the monitoring.coreos.com CRD is not installed.
	ErrServiceMonitorCRDMissing = errors.New("ServiceMonitor CRD is not installed; install the Prometheus Operator or set monitoring.serviceMonitor.enabled=false")

	// ErrServiceMonitorCreate is returned when creating or updating the
	// ServiceMonitor resource fails.
	ErrServiceMonitorCreate = errors.New("could not create serviceMonitor")

	// ErrServiceMonitorDelete is returned when deleting the ServiceMonitor
	// resource fails.
	ErrServiceMonitorDelete = errors.New("could not delete serviceMonitor")
)
