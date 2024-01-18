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

package trillian

import (
	"context"

	"github.com/securesign/operator/controllers/common/action"

	"github.com/securesign/operator/client"
	p "github.com/securesign/operator/controllers/common/operator/predicate"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

// TrillianReconciler reconciles a Trillian object
type TrillianReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=trillians,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=trillians/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=trillians/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Trillian object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *TrillianReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Fetch the Trillian instance
	var instance rhtasv1alpha1.Trillian
	log := ctrllog.FromContext(ctx)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	target := instance.DeepCopy()
	actions := []action.Action[rhtasv1alpha1.Trillian]{
		NewCreateAction(),
		NewInitializeAction(),
		NewWaitAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)
		a.InjectLogger(log)

		if a.CanHandle(target) {
			newTarget, err := a.Handle(ctx, target)
			if err != nil {
				if newTarget != nil {
					_ = r.Status().Update(ctx, newTarget)
				}
				return reconcile.Result{}, err
			}

			if newTarget != nil {
				if err := r.Status().Update(ctx, newTarget); err != nil {
					return reconcile.Result{}, err
				}
			}
			break
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrillianReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Trillian{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, p.StatusChangedPredicate{}))).
		Owns(&v1.Deployment{}, builder.WithPredicates(
			// ignore create events
			predicate.Funcs{CreateFunc: func(event event.CreateEvent) bool {
				return false
			}},
		)).
		Complete(r)
}
