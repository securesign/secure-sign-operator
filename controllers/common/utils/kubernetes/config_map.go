package kubernetes

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func InitConfigmap(namespace string, name string, labels map[string]string, data map[string]string) *corev1.ConfigMap {
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
func FindConfigMap(ctx context.Context, c client.Client, namespace string, label string) (*corev1.ConfigMap, error) {
	list := &corev1.ConfigMapList{}

	selector, err := labels.Parse(label)
	listOptions := &client.ListOptions{
		LabelSelector: selector,
	}

	err = c.List(ctx, list, client.InNamespace(namespace), listOptions)

	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		return nil, errors.New("duplicate resource")
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	return nil, nil
}
