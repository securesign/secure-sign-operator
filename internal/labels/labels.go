package labels

import "github.com/securesign/operator/internal/constants"

const (
	LabelNamespace = "rhtas.redhat.com"
	LabelResource  = LabelNamespace + "/resource"

	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppComponent = "app.kubernetes.io/component"
	LabelAppPartOf    = "app.kubernetes.io/part-of"
	LabelAppManagedBy = "app.kubernetes.io/managed-by"
	LabelAppNamespace = "app.kubernetes.io/instance-namespace"
)

func For(component, name, instance string) map[string]string {
	labels := ForComponent(component, instance)
	labels[LabelAppName] = name

	return labels
}

func ForComponent(component, instance string) map[string]string {
	return map[string]string{
		LabelAppInstance:  instance,
		LabelAppComponent: component,
		LabelAppPartOf:    constants.AppName,
		LabelAppManagedBy: "controller-manager",
	}
}

func ForResource(component, name, instance, resource string) map[string]string {
	labels := For(component, name, instance)
	labels[LabelResource] = resource
	return labels
}
