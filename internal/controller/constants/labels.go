package constants

const (
	LabelNamespace = "rhtas.redhat.com"
)

func LabelsFor(component, name, instance string) map[string]string {
	labels := LabelsForComponent(component, instance)
	labels["app.kubernetes.io/name"] = name

	return labels
}

func LabelsForComponent(component, instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":   instance,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/part-of":    "trusted-artifact-signer",
		"app.kubernetes.io/managed-by": "controller-manager",
	}
}

func LabelsRHTAS() map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":    "trusted-artifact-signer",
		"app.kubernetes.io/managed-by": "controller-manager",
	}
}
