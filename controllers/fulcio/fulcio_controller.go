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

package fulcio

import (
	"context"
	"errors"

	"github.com/securesign/operator/controllers/fulcio/actions"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/api/networking/v1"
	"k8s.io/client-go/tools/record"

	"github.com/securesign/operator/controllers/common/action"

	v1 "k8s.io/api/apps/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

var requeueError = errors.New("requeue the reconcile key")

// FulcioReconciler reconciles a Fulcio object
type FulcioReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=fulcios,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=fulcios/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=fulcios/finalizers,verbs=update
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=secrets,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Fulcio object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *FulcioReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	var instance rhtasv1alpha1.Fulcio
	log := ctrllog.FromContext(ctx)
	log.V(1).Info("Reconciling Fulcio", "request", req)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		if k8sErrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	target := instance.DeepCopy()
	acs := []action.Action[rhtasv1alpha1.Fulcio]{
		actions.NewToPendingPhaseAction(),
		actions.NewGenerateCertAction(),
		actions.NewRBACAction(),
		actions.NewServerConfigAction(),
		actions.NewDeployAction(),
		actions.NewCreateMonitorAction(),
		actions.NewServiceAction(),
		actions.NewIngressAction(),
		actions.NewToInitializeAction(),
		actions.NewInitializeAction(),
	}

	for _, a := range acs {
		a.InjectClient(r.Client)
		a.InjectLogger(log.WithName(a.Name()))
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
func (r *FulcioReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Fulcio{}).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Owns(&v13.Ingress{}).
		Complete(r)
}
