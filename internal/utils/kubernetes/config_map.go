package kubernetes

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetConfigMap(ctx context.Context, client client.Client, namespace, secretName string) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap

	err := client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, &cm)

	if err != nil {
		return nil, err
	}
	return &cm, nil
}

func ListConfigMaps(ctx context.Context, c client.Client, namespace string, labelSelector string) (*metav1.PartialObjectMetadataList, error) {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}

	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvk)

	err := FindByLabelSelector(ctx, c, list, namespace, labelSelector)

	if err != nil {
		return nil, err
	}
	return list, nil
}
