package labels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/securesign/operator/internal/controller/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func Remove(ctx context.Context, object *metav1.PartialObjectMetadata, c client.Client, label string) error {
	object.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	patch, err := json.Marshal([]map[string]string{
		{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/labels/%s", strings.ReplaceAll(label, "/", "~1")),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	err = c.Patch(ctx, object, client.RawPatch(types.JSONPatchType, patch))
	if err != nil {
		return fmt.Errorf("unable to remove '%s' label from secret: %w", label, err)
	}

	return nil
}
