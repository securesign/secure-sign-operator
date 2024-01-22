package trillianUtils

import (
	"context"
	"errors"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FindTrillian(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (*rhtasv1alpha1.Trillian, error) {
	list := &rhtasv1alpha1.TrillianList{}
	err := cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		return nil, errors.New("dupplicit resource")
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	// try to find any resource in namespace
	err = cli.List(ctx, list, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 1 {
		return nil, errors.New("dupplicit resource")
	}

	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	return nil, errors.New("no resource found")

}
