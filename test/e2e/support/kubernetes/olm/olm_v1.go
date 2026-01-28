package olm

import (
	"context"
	"fmt"

	coreV1 "k8s.io/api/core/v1"
	rbacV1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterExtensionWrapper struct {
	*unstructured.Unstructured
}

func (e *clusterExtensionWrapper) Unwrap() client.Object {
	return e.Unstructured
}

func (e *clusterExtensionWrapper) IsReady(_ context.Context, _ client.Client) bool {
	return meta.IsStatusConditionTrue(conditions(e.Object), "Installed")
}

func (e *clusterExtensionWrapper) GetVersion(_ context.Context, _ client.Client) string {
	version, _, _ := unstructured.NestedString(e.Object, "status", "install", "bundle", "version")
	return version
}

type clusterCatalogSource struct {
	*unstructured.Unstructured
}

func (c *clusterCatalogSource) Unwrap() client.Object {
	return c.Unstructured
}

func (c *clusterCatalogSource) IsReady(_ context.Context, _ client.Client) bool {
	return meta.IsStatusConditionTrue(conditions(c.Object), "Serving")
}

func (c *clusterCatalogSource) UpdateSourceImage(s string) {
	_ = unstructured.SetNestedField(c.Object, s, "spec", "source", "image", "ref")
}

func OlmV1Installer(ctx context.Context, cli client.Client, catalogImage, ns, packageName, channel string) (Extension, ExtensionSource, error) {
	sa := &coreV1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-installer", packageName),
			Namespace: ns,
		},
	}

	crb := &rbacV1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-installer-%s", packageName, ns),
		},
		RoleRef: rbacV1.RoleRef{
			APIGroup: coreV1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacV1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: ns}},
	}

	selector := map[string]string{"catalog": "test"}

	cs := &clusterCatalogSource{
		Unstructured: &unstructured.Unstructured{},
	}
	cs.SetKind("ClusterCatalog")
	cs.SetAPIVersion("olm.operatorframework.io/v1")
	cs.SetName(fmt.Sprintf("%s-catalog", packageName))
	cs.SetLabels(selector)
	if err := unstructured.SetNestedMap(cs.Object, map[string]interface{}{
		"availabilityMode": "Available",
		"priority":         int64(-300),
		"source": map[string]interface{}{
			"type": "Image",
			"image": map[string]interface{}{
				"ref": catalogImage,
			},
		},
	}, "spec"); err != nil {
		return nil, nil, err
	}

	es := &clusterExtensionWrapper{
		Unstructured: &unstructured.Unstructured{},
	}
	es.SetKind("ClusterExtension")
	es.SetAPIVersion("olm.operatorframework.io/v1")
	es.SetName(packageName)

	if err := unstructured.SetNestedMap(es.Object, map[string]interface{}{
		"namespace":      ns,
		"serviceAccount": map[string]interface{}{"name": sa.Name},
		"source": map[string]interface{}{
			"sourceType": "Catalog",
			"catalog": map[string]interface{}{
				"packageName": packageName,
			},
		},
	}, "spec"); err != nil {
		return nil, nil, err
	}
	if err := unstructured.SetNestedStringMap(es.Object, selector, "spec", "source", "catalog", "selector", "matchLabels"); err != nil {
		return nil, nil, err
	}
	if err := unstructured.SetNestedStringSlice(es.Object, []string{channel}, "spec", "source", "catalog", "channels"); err != nil {
		return nil, nil, err
	}

	for _, obj := range []client.Object{sa, crb, es.Unwrap(), cs.Unwrap()} {
		if err := cli.Create(ctx, obj); err != nil {
			return nil, nil, err
		}
	}

	return es, cs, nil
}

func conditions(object map[string]interface{}) []metav1.Condition {
	var (
		conditionsSlice []interface{}
		found           bool
		err             error
	)
	if conditionsSlice, found, err = unstructured.NestedSlice(object, "status", "conditions"); !found || err != nil {
		return nil
	}

	jsonBytes, err := json.Marshal(conditionsSlice)
	if err != nil {
		return nil
	}

	var typedConditions []metav1.Condition
	if err := json.Unmarshal(jsonBytes, &typedConditions); err != nil {
		return nil
	}
	return typedConditions
}
