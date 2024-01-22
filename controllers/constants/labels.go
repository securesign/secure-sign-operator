package constants

const (
	LabelNamespace = "rhtas.redhat.com"
	//DiscoverableByTUFKeyLabel = LabelNamespace + "/tuf-key"
	TufLabelNamespace = "tuf." + LabelNamespace
)

func TufDiscoverableSecretLabel(name string, key string) map[string]string {
	return map[string]string{
		TufLabelNamespace + "/" + name: key,
	}
}
func LabelsFor(component, name, instance string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/instance":   instance,
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/part-of":    "trusted-artifact-signer",
		"app.kubernetes.io/managed-by": "controller-manager",
	}
}
