package controller

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ServiceRefWatch(cl client.Client, listObj client.ObjectList, getRef func(client.Object) rhtasv1.ServiceReference) handler.MapFunc {
	return func(ctx context.Context, object client.Object) []reconcile.Request {
		list := listObj.DeepCopyObject().(client.ObjectList)
		if err := cl.List(ctx, list); err != nil {
			return nil
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			return nil
		}
		var requests []reconcile.Request
		for _, raw := range items {
			item, ok := raw.(client.Object)
			if !ok {
				continue
			}
			sr := getRef(item)
			switch {
			// serviceRef binding
			case sr.Ref != nil:
				if sr.Ref.Name == object.GetName() && sr.Ref.Namespace == object.GetNamespace() {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: item.GetNamespace(), Name: item.GetName()}})
				}
			// autodiscovery
			case sr.URL == "":
				if item.GetNamespace() == object.GetNamespace() {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: item.GetNamespace(), Name: item.GetName()}})
				}
			}
		}
		return requests
	}
}
