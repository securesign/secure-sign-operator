package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/securesign/operator/internal/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	rhtasv1 "github.com/securesign/operator/api/v1"
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

func GetSecretData(client client.Client, namespace string, selector *rhtasv1.SecretKeySelector) ([]byte, error) {
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

var secretGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}

// ExistsSecret checks whether a Secret exists by name using a metadata-only GET
// (no secret data is fetched).
//
// Returns:
//   - (true, nil)  — secret exists
//   - (false, nil) — secret confirmed not found
//   - (false, err) — transient/permission error
func ExistsSecret(ctx context.Context, c client.Client, namespace, name string) (bool, error) {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(secretGVK)
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func FindSecret(ctx context.Context, c client.Client, namespace string, label string) (*metav1.PartialObjectMetadata, error) {
	list, err := ListSecrets(ctx, c, namespace, label)
	if err != nil {
		return nil, err
	}

	switch len(list.Items) {
	case 1:
		return &list.Items[0], nil
	case 0:
		return nil, apierrors.NewNotFound(schema.GroupResource{
			Group:    secretGVK.Group,
			Resource: secretGVK.Kind,
		}, "")
	default:
		return nil, errors.New("duplicate resource")
	}
}

func ListSecrets(ctx context.Context, c client.Client, namespace string, labelSelector string) (*metav1.PartialObjectMetadataList, error) {
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(secretGVK)

	err := FindByLabelSelector(ctx, c, list, namespace, labelSelector)

	if err != nil {
		return nil, err
	}
	return list, nil

}

var (
	// ErrImmutableSecretDataMismatch is returned when an attempt is made to update
	// the data of a Kubernetes Secret that has Immutable: true.
	ErrImmutableSecretDataMismatch = errors.New("can't update immutable Secret data")

	// ErrImmutableSecretMutability is returned when an attempt is made to change
	// a Secret's Immutable field from true to false.
	ErrImmutableSecretMutability = errors.New("can't update Secret mutability")
)

func EnsureSecretData(immutable bool, data map[string][]byte) func(secret *corev1.Secret) error {
	return func(instance *corev1.Secret) error {
		switch {
		case !utils.OptionalBool(instance.Immutable):
		case !reflect.DeepEqual(instance.Data, data):
			return fmt.Errorf("%w", ErrImmutableSecretDataMismatch)
		case utils.OptionalBool(instance.Immutable) && !immutable:
			return fmt.Errorf("%w", ErrImmutableSecretMutability)
		}
		instance.Immutable = utils.Pointer(immutable)
		instance.Data = data
		return nil
	}
}
