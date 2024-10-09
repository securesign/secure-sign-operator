package kubernetes

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateConfigmap(namespace string, name string, labels map[string]string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
	}
}

func CreateImmutableConfigmap(namePrefix string, namespace string, labels map[string]string, data map[string]string) *corev1.ConfigMap {
	immutable := true
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
			Namespace:    namespace,
			Labels:       labels,
		},
		Data:      data,
		Immutable: &immutable,
	}
}

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

func FindConfigMap(ctx context.Context, c client.Client, namespace string, labelSelector string) (*metav1.PartialObjectMetadata, error) {
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
	if len(list.Items) > 1 {
		return nil, errors.New("duplicate resource")
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{
		Group:    gvk.Group,
		Resource: gvk.Kind,
	}, "")
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
