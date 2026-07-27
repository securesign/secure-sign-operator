package utils

import (
	"context"
	"fmt"

	v1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/serviceresolver"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrGetServiceFailed    = fmt.Errorf("failed to get service")
	ErrAutodiscoveryFailed = fmt.Errorf("failed to autodiscovery service")
)

func ResolveInternalServiceUrl(ctx context.Context, cl client.Client, serviceRef v1.ServiceReference, instanceNamespace string, instance client.Object) (string, error) {
	if serviceRef.URL != "" {
		return serviceRef.URL, nil
	}
	if serviceRef.Ref != nil && serviceRef.Ref.Name != "" {
		if err := cl.Get(ctx, types.NamespacedName{Namespace: serviceRef.Ref.Namespace, Name: serviceRef.Ref.Name}, instance); err != nil {
			return "", fmt.Errorf("%w: %w", ErrGetServiceFailed, err)
		}
		return serviceresolver.Resolve(instance)
	}

	// Autoload service from list of objects (backwards compatibility)
	var (
		listObject client.ObjectList
		err        error
	)
	if listObject, err = objectAsList(cl, instance); err != nil {
		return "", err
	}
	if instance, err = autoloadService(ctx, cl, instanceNamespace, listObject); err != nil {
		return "", err
	}
	return serviceresolver.Resolve(instance)
}

func autoloadService(ctx context.Context, cl client.Client, namespace string, list client.ObjectList) (client.Object, error) {
	if err := cl.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAutodiscoveryFailed, err)
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAutodiscoveryFailed, err)
	}
	switch len(items) {
	case 0:
		return nil, fmt.Errorf("%w: no %T found in namespace %s", ErrAutodiscoveryFailed, list, namespace)
	case 1:
		obj, ok := items[0].(client.Object)
		if !ok {
			return nil, fmt.Errorf("%w: %T does not implement client.Object", ErrAutodiscoveryFailed, items[0])
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("%w: found %d instances in namespace %s", ErrAutodiscoveryFailed, len(items), namespace)
	}
}

func objectAsList(cl client.Client, instance client.Object) (client.ObjectList, error) {
	gvks, _, err := cl.Scheme().ObjectKinds(instance)
	if err != nil {
		return nil, fmt.Errorf("resolving object kind: %w", err)
	}
	if len(gvks) == 0 {
		return nil, fmt.Errorf("no GVK registered for %T", instance)
	}
	listGVK := gvks[0]
	listGVK.Kind += "List"
	obj, err := cl.Scheme().New(listGVK)
	if err != nil {
		return nil, fmt.Errorf("creating list for %s: %w", listGVK.Kind, err)
	}
	list, ok := obj.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("%s does not implement client.ObjectList", listGVK.Kind)
	}
	return list, nil
}
