package ctlog

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

// CTlogReconciler reconciles a CTlog object
type CTlogReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *CTlogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var instance rhtasv1alpha1.CTlog

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
	actions := []Action{
		NewInitializeAction(),
		NewWaitAction(),
	}

	for _, a := range actions {
		a.InjectClient(r.Client)

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
func (r *CTlogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.CTlog{}).
		Owns(&v1.Deployment{}).
		Complete(r)
}
