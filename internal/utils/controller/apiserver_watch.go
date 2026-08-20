package controller

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	crpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// watchesRegistrar is the subset of builder.Builder used to register a watch.
// *builder.Builder satisfies it, and it allows the guard in WatchAPIServer to be
// unit-tested with a spy without standing up a manager.
type watchesRegistrar interface {
	Watches(object client.Object, eventHandler handler.EventHandler, opts ...builder.WatchesOption) *builder.Builder
}

// EnqueueAllOnClusterConfigChange returns a handler.MapFunc that enqueues a
// reconcile request for every object contained in listObj.
//
// It is used to fan out a change on a cluster-scoped singleton (for example the
// config.openshift.io APIServer/cluster object that carries the cluster-wide TLS
// security profile) to all instances of a namespaced CR kind, so each instance
// re-reconciles and re-reads the cluster-wide configuration. The triggering
// object itself is ignored; the full list of CRs is always enqueued.
func EnqueueAllOnClusterConfigChange(cl client.Client, listObj client.ObjectList) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx).WithName("APIServerWatch")
		list := listObj.DeepCopyObject().(client.ObjectList)
		if err := cl.List(ctx, list); err != nil {
			log.V(1).Info("failed to list CRs for cluster config change, skipping enqueue", "error", err)
			return nil
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			log.V(1).Info("failed to extract CR list for cluster config change, skipping enqueue", "error", err)
			return nil
		}
		var requests []reconcile.Request
		for _, raw := range items {
			item, ok := raw.(client.Object)
			if !ok {
				continue
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: item.GetNamespace(), Name: item.GetName()},
			})
		}
		if len(requests) > 0 {
			log.V(1).Info("cluster TLS security profile changed, enqueuing reconcile for all CRs",
				"trigger", obj.GetName(), "count", len(requests))
		}
		return requests
	}
}

// WatchAPIServer registers a watch on the cluster-wide config.openshift.io
// APIServer singleton that re-reconciles every CR in listObj when it changes,
// so TLS-sensitive outputs converge on the new cluster TLS security profile.
//
// The watch is only registered on OpenShift: the APIServer CRD does not exist on
// vanilla Kubernetes and watching a missing CRD makes the manager fail at
// startup. ResourceVersionChangedPredicate suppresses no-op cache resyncs so
// unrelated cache events do not trigger a reconcile storm.
func WatchAPIServer(b watchesRegistrar, cl client.Client, listObj client.ObjectList) {
	if !kubernetes.IsOpenShift() {
		return
	}
	b.Watches(&configv1.APIServer{}, handler.EnqueueRequestsFromMapFunc(
		EnqueueAllOnClusterConfigChange(cl, listObj),
	), builder.WithPredicates(crpredicate.ResourceVersionChangedPredicate{}))
}
