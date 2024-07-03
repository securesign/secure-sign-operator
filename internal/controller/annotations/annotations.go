package annotations

const (
	// PausedReconciliation Annotation used to pause resource reconciliation
	PausedReconciliation = "rhtas.redhat.com/pause-reconciliation"

	// Metrics Annotation is used to control the sending of analytic metrics of the installed services managed by the operator.
	Metrics = "rhtas.redhat.com/metrics"

	// TrustedCA Annotation to specify name of ConfigMap with additional bundle of trusted CA
	TrustedCA = "rhtas.redhat.com/trusted-ca"
)

var inheritable = []string{
	TrustedCA,
}

func FilterInheritable(annotations map[string]string) map[string]string {
	result := make(map[string]string, 0)
	for key, value := range annotations {
		for _, ia := range inheritable {
			if key == ia {
				result[key] = value
			}
		}
	}
	return result
}
