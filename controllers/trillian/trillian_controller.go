package trillian

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (r *TrillianReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Fetch the Trillian instance
	var instance rhtasv1alpha1.Trillian

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
func (r *TrillianReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Trillian{}).
		Owns(&v1.Deployment{}).
		Complete(r)
}
