package console

import (
	"context"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/transitions"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller"
	"k8s.io/apimachinery/pkg/types"

	olpredicate "github.com/operator-framework/operator-lib/predicate"
	"github.com/securesign/operator/internal/controller/console/actions"
	consoleapi "github.com/securesign/operator/internal/controller/console/actions/api"
	"github.com/securesign/operator/internal/controller/console/actions/ui"
	v12 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"

	v1 "k8s.io/api/apps/v1"
	v13 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1 "github.com/securesign/operator/api/v1"
	tasPredicate "github.com/securesign/operator/internal/controller/predicate"
)

type consoleReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	recorder events.EventRecorder
}

func NewReconciler(c client.Client, scheme *runtime.Scheme, recorder events.EventRecorder) controller.Controller {
	return &consoleReconciler{
		Client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=consoles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=consoles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=consoles/finalizers,verbs=update

func (r *consoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var instance rhtasv1.Console
	log := ctrllog.FromContext(ctx)

	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	var namespace v12.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !namespace.DeletionTimestamp.IsZero() {
		log.Info("namespace is marked for deletion, stopping reconciliation", "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	target := instance.DeepCopy()
	conditionSupplier := func(_ *rhtasv1.Console) []string {
		return []string{actions.ApiCondition, actions.UICondition}
	}
	actionList := []action.Action[*rhtasv1.Console]{
		transitions.NewToPendingPhaseAction[*rhtasv1.Console](),
		transitions.NewEnsureConditionsAction[*rhtasv1.Console](conditionSupplier),

		consoleapi.NewTlsAction(),

		transitions.NewToCreatePhaseAction[*rhtasv1.Console](),
		consoleapi.NewRBACAction(),
		ui.NewRBACAction(),

		consoleapi.NewDeployAction(),
		consoleapi.NewCreateServiceAction(),

		ui.NewDeployAction(),
		ui.NewCreateServiceAction(),
		ui.NewIngressAction(),
		ui.NewStatusUrlAction(),

		transitions.NewToInitializePhaseAction[*rhtasv1.Console](),

		consoleapi.NewRolloutCheckAction(),
		ui.NewRolloutCheckAction(),

		transitions.NewToReadyPhaseAction[*rhtasv1.Console](),
	}

	for _, a := range actionList {
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

func (r *consoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pause, err := olpredicate.NewPause[client.Object](annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1.Console{}, builder.WithPredicates(tasPredicate.ConfigurationChangedOnFailurePredicate[*rhtasv1.Console]())).
		Owns(&v1.Deployment{}).
		Owns(&v12.Service{}).
		Owns(&v13.Ingress{}).
		Complete(r)
}
