package utils

import (
	"context"
	"errors"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FindFulcio(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (*rhtasv1alpha1.Fulcio, error) {
	list := &rhtasv1alpha1.FulcioList{}
	err := cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels), client.Limit(1))
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	// try to find any resource in namespace
	err = cli.List(ctx, list, client.InNamespace(namespace), client.Limit(1))
	if err != nil {
		return nil, err
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	return nil, errors.New("component not found")
}
