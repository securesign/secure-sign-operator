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

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/transitions"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller"
	"github.com/securesign/operator/internal/controller/tuf/actions"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const DebugLevel int = 1

// tufReconciler reconciles a Tuf object
type tufReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

func NewReconciler(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) controller.Controller {
	return &tufReconciler{
		Client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
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
func (r *tufReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rlog := log.FromContext(ctx).WithName("controller").WithName("tuf")

	// Fetch the Tuf instance
	instance := &rhtasv1alpha1.Tuf{}

	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the namespace
	var namespace v12.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the namespace is marked for deletion
	if !namespace.DeletionTimestamp.IsZero() {
		rlog.Info("namespace is marked for deletion, stopping reconciliation", "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	target := instance.DeepCopy()
	acs := []action.Action[*rhtasv1alpha1.Tuf]{
		transitions.NewToPendingPhaseAction[*rhtasv1alpha1.Tuf](func(tuf *rhtasv1alpha1.Tuf) []string {
			conditions := make([]string, len(tuf.Spec.Keys))
			for i, k := range tuf.Spec.Keys {
				conditions[i] = k.Name
			}
			conditions = append(conditions, constants.RepositoryCondition)
			return conditions
		}),

		actions.NewResolveKeysAction(),
		transitions.NewToCreatePhaseAction[*rhtasv1alpha1.Tuf](),
		actions.NewRBACInitJobAction(),
		actions.NewRBACAction(),
		actions.NewCreatePvcAction(),
		actions.NewInitJobAction(),
		actions.NewDeployAction(),
		actions.NewServiceAction(),
		actions.NewIngressAction(),
		actions.NewStatusUrlAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1alpha1.Tuf](),

		actions.NewInitializeAction(),

		transitions.NewToReadyPhaseAction[*rhtasv1alpha1.Tuf](),
	}

	for _, a := range acs {
		a.InjectClient(r.Client)
		a.InjectLogger(rlog.WithName(a.Name()))
		a.InjectRecorder(r.recorder)

		if a.CanHandle(ctx, target) {
			rlog.V(2).Info("Executing " + a.Name())
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *tufReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var (
		err error
	)

	// Filter out with the pause annotation.
	pause, err := olpredicate.NewPause[client.Object](annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.Tuf{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Owns(&v13.Ingress{}).
		Complete(r)
}
