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

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/tuf/actions"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const DebugLevel int = 1

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
	rlog.V(1).Info("Reconciling TUF", "request", req)

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

	target := instance.DeepCopy()
	acs := []action.Action[rhtasv1alpha1.Tuf]{
		actions.NewToPendingPhaseAction(),

		actions.NewResolveKeysAction(),
		actions.NewRBACAction(),
		actions.NewDeployAction(),
		actions.NewServiceAction(),
		actions.NewIngressAction(),

		actions.NewToInitializePhaseAction(),

		actions.NewInitializeAction(),
	}

	for _, a := range acs {
		a.InjectClient(r.Client)
		a.InjectLogger(rlog.WithName(a.Name()))
		a.InjectRecorder(r.Recorder)

		if a.CanHandle(target) {
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TufReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Tuf{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Owns(&v13.Ingress{}).
		Complete(r)
}
