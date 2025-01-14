package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/securesign/operator/internal/controller/common/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecret(client client.Client, namespace, secretName string) (*corev1.Secret, error) {
	var secret corev1.Secret

	err := client.Get(context.TODO(), types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, &secret)

	if err != nil {
		return nil, err
	}
	return &secret, nil
}

func GetSecretData(client client.Client, namespace string, selector *rhtasv1alpha1.SecretKeySelector) ([]byte, error) {
	if selector != nil && selector.Name != "" && selector.Key != "" {
		secret, err := GetSecret(client, namespace, selector.Name)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve secret %s: %w", selector.Name, err)
		}
		if val, ok := secret.Data[selector.Key]; ok {
			return val, nil
		} else {
			return nil, fmt.Errorf("could not retrieve %s secret's key %s: %w", selector.Name, selector.Key, err)
		}
	}
	return nil, nil
}

func FindSecret(ctx context.Context, c client.Client, namespace string, label string) (*metav1.PartialObjectMetadata, error) {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	}

	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvk)

	err := FindByLabelSelector(ctx, c, list, namespace, label)

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

func ListSecrets(ctx context.Context, c client.Client, namespace string, labelSelector string) (*metav1.PartialObjectMetadataList, error) {
	gvk := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	}

	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(gvk)

	err := FindByLabelSelector(ctx, c, list, namespace, labelSelector)

	if err != nil {
		return nil, err
	}
	return list, nil

}

func EnsureSecretData(immutable bool, data map[string][]byte) func(secret *corev1.Secret) error {
	return func(instance *corev1.Secret) error {
		switch {
		case !utils.OptionalBool(instance.Immutable):
		case !reflect.DeepEqual(instance.Data, data):
			return fmt.Errorf("can't update immutable Secret data")
		case utils.OptionalBool(instance.Immutable) && !immutable:
			return fmt.Errorf("can't make update Secret mutability")
		}
		instance.Immutable = utils.Pointer(immutable)
		instance.Data = data
		return nil
	}
}
