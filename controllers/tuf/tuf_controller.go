/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tuf

import (
	"context"
	"errors"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/client"
	"github.com/securesign/operator/controllers/common/action"
	p "github.com/securesign/operator/controllers/common/operator/predicate"
	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	rclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

const DebugLevel int = 1

var requeueError = errors.New("requeue the reconcile key")

// TufReconciler reconciles a Tuf object
type TufReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=tufs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=tufs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=tufs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Tuf object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *TufReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rlog := log.FromContext(ctx).WithName("controller").WithName("tuf")
	rlog.V(DebugLevel).Info("Reconciling TUF", "request", req)

	// Fetch the Tuf instance
	instance := &rhtasv1alpha1.Tuf{}

	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		} else {
			// Error reading the object - requeue the request.
			return reconcile.Result{}, err
		}
	}

	if instance.Status.Conditions == nil || len(instance.Status.Conditions) == 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   string(rhtasv1alpha1.PhaseReady),
			Status: metav1.ConditionUnknown,
			Reason: string(rhtasv1alpha1.PhasePending)})
		if err := r.Status().Update(ctx, instance); err != nil {
			rlog.Error(err, "Failed to update TUF status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the Tuf Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		time.Sleep(time.Second)
		if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
			rlog.Error(err, "Failed to re-fetch TUF")
			return ctrl.Result{}, err
		}
	}

	target := instance.DeepCopy()
	actions := []action.Action[rhtasv1alpha1.Tuf]{
		NewPendingAction(),
		NewReconcileAction(),
		NewWaitAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)
		a.InjectLogger(rlog.WithName(a.Name()))
		a.InjectRecorder(r.Recorder)

		if a.CanHandle(target) {
			newTarget, err := a.Handle(ctx, target)
			if err != nil {
				if newTarget != nil {
					_ = r.Status().Update(ctx, newTarget)
				}
				if errors.Is(err, requeueError) {
					return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
				}
				return reconcile.Result{}, err
			}

			if newTarget != nil {
				if err = r.Status().Update(ctx, newTarget); err != nil {
					return reconcile.Result{}, err
				}
			}
			break
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TufReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Tuf{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, p.StatusChangedPredicate{}))).
		Owns(&v1.Deployment{}, builder.WithPredicates(
			// ignore create events
			predicate.Funcs{CreateFunc: func(event event.CreateEvent) bool {
				return false
			}},
		)).
		Watches(&rhtasv1alpha1.Rekor{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a rclient.Object) []reconcile.Request {
			var requests []reconcile.Request
			t, ok := a.(*rhtasv1alpha1.Rekor)
			if !ok {
				return requests
			}
			rekors := &rhtasv1alpha1.RekorList{}
			if err := mgr.GetClient().List(ctx, rekors, rclient.MatchingLabels(t.Labels), rclient.InNamespace(t.Namespace)); err != nil {
				return requests
			}

			for _, i := range rekors.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: i.Namespace,
						Name:      i.Name,
					},
				})
			}
			return requests
		})).
		Watches(&rhtasv1alpha1.Fulcio{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a rclient.Object) []reconcile.Request {
			var requests []reconcile.Request
			t, ok := a.(*rhtasv1alpha1.Fulcio)
			if !ok {
				return requests
			}
			fulcios := &rhtasv1alpha1.FulcioList{}
			if err := mgr.GetClient().List(ctx, fulcios, rclient.MatchingLabels(t.Labels), rclient.InNamespace(t.Namespace)); err != nil {
				return requests
			}

			for _, i := range fulcios.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: i.Namespace,
						Name:      i.Name,
					},
				})
			}
			return requests
		})).
		Complete(r)
}
