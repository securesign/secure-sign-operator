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

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/transitions"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller"
	"k8s.io/apimachinery/pkg/types"

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/controller/trillian/actions/db"
	"github.com/securesign/operator/internal/controller/trillian/actions/logserver"
	"github.com/securesign/operator/internal/controller/trillian/actions/logsigner"
	v12 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

// trillianReconciler reconciles a Trillian object
type trillianReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

func NewReconciler(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) controller.Controller {
	return &trillianReconciler{
		Client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
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
func (r *trillianReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the Trillian instance
	var instance rhtasv1alpha1.Trillian
	log := ctrllog.FromContext(ctx)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the namespace
	var namespace v12.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the namespace is marked for deletion
	if !namespace.DeletionTimestamp.IsZero() {
		log.Info("namespace is marked for deletion, stopping reconciliation", "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	target := instance.DeepCopy()
	actions := []action.Action[*rhtasv1alpha1.Trillian]{
		transitions.NewToPendingPhaseAction[*rhtasv1alpha1.Trillian](func(t *rhtasv1alpha1.Trillian) []string {
			return []string{actions.ServerCondition, actions.SignerCondition, actions.DbCondition}
		}),

		logserver.NewTlsAction(),
		logsigner.NewTlsAction(),
		db.NewTlsAction(),

		transitions.NewToCreatePhaseAction[*rhtasv1alpha1.Trillian](),
		logserver.NewRBACAction(),
		logsigner.NewRBACAction(),
		db.NewRBACAction(),

		db.NewHandleSecretAction(),
		db.NewCreatePvcAction(),
		db.NewDeployAction(),
		db.NewCreateServiceAction(),

		logserver.NewDeployAction(),
		logserver.NewCreateServiceAction(),
		logserver.NewCreateMonitorAction(),

		logsigner.NewDeployAction(),
		logsigner.NewCreateServiceAction(),
		logsigner.NewCreateMonitorAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1alpha1.Trillian](),

		db.NewInitializeAction(),
		logserver.NewInitializeAction(),
		logsigner.NewInitializeAction(),

		transitions.NewToReadyPhaseAction[*rhtasv1alpha1.Trillian](),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)
		a.InjectLogger(log.WithName(a.Name()))
		a.InjectRecorder(r.recorder)

		if a.CanHandle(ctx, target) {
			log.V(2).Info("Executing " + a.Name())
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *trillianReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter out with the pause annotation.
	pause, err := olpredicate.NewPause[client.Object](annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.Trillian{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Complete(r)
}
