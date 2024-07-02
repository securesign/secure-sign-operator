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

package tsa

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/action/transitions"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/networking/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TimestampAuthorityReconciler reconciles a TimestampAuthority object
type TimestampAuthorityReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=timestampauthorities,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=timestampauthorities/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=timestampauthorities/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TimestampAuthority object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *TimestampAuthorityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var instance rhtasv1alpha1.TimestampAuthority
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Reconciling Timestamp Authority", "request", req)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
	}

	target := instance.DeepCopy()
	actions := []action.Action[*rhtasv1alpha1.TimestampAuthority]{
		transitions.NewToPendingPhaseAction[*rhtasv1alpha1.TimestampAuthority](func(ta *rhtasv1alpha1.TimestampAuthority) []string {
			components := []string{actions.TSASignerCondition, actions.TSAServerCondition}
			return components
		}),
		actions.NewHandleCertsAction(),
		transitions.NewToCreatePhaseAction[*rhtasv1alpha1.TimestampAuthority](),
		actions.NewRBACAction(),
		actions.NewDeployAction(),
		actions.NewServiceAction(),
		actions.NewIngressAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1alpha1.TimestampAuthority](),
		actions.NewInitializeAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)
		a.InjectLogger(log.WithName(a.Name()))
		a.InjectRecorder(r.Recorder)

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
func (r *TimestampAuthorityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter out with the pause annotation.
	pause, err := olpredicate.NewPause(annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.TimestampAuthority{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Owns(&v13.Ingress{}).
		Complete(r)
}
