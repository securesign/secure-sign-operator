package kubernetes

import (
	"context"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetRoute(ctx context.Context, cli client.Client, namespace string, ingressLabels map[string]string) (*routev1.Route, error) {
	if !IsOpenShift() {
		return nil, fmt.Errorf("not running in an openshift environment")
	}
	labelSelector := labels.SelectorFromSet(labels.Set(ingressLabels))
	list := &routev1.RouteList{}
	listOptions := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := cli.List(ctx, list, client.InNamespace(namespace), listOptions); err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no route found with matching labels")
	}
	return &list.Items[0], nil
}
