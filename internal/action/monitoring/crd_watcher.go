package monitoring

import (
	"context"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ServiceMonitorCRDName is the name of the ServiceMonitor CRD
	ServiceMonitorCRDName = "servicemonitors.monitoring.coreos.com"
)

// CRDWatcherReconciler watches CRD resources to detect
// when the Prometheus Operator ServiceMonitor CRD is installed or removed
type CRDWatcherReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Registry *ServiceMonitorRegistry
}

// NewCRDWatcher creates a new CRD watcher reconciler
func NewCRDWatcher(c client.Client, scheme *runtime.Scheme, registry *ServiceMonitorRegistry) (*CRDWatcherReconciler, error) {
	return &CRDWatcherReconciler{
		Client:   c,
		Scheme:   scheme,
		Registry: registry,
	}, nil
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconcile processes CRD changes for the ServiceMonitor CRD
func (r *CRDWatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != ServiceMonitorCRDName {
		return reconcile.Result{}, nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Detected ServiceMonitor CRD change")

	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := r.Get(ctx, req.NamespacedName, crd)

	available := false
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get CRD")
			return reconcile.Result{}, err
		}
		logger.Info("ServiceMonitor CRD not found - Prometheus Operator not installed")
	} else {
		available = isCRDEstablished(crd)
		if !available {
			logger.Info("ServiceMonitor CRD found but not established yet")
		}
	}

	previouslyAvailable := r.Registry.IsAPIAvailable()
	r.Registry.SetAPIAvailable(available)

	if available != previouslyAvailable {
		if available {
			logger.Info("ServiceMonitor CRD is now available")
			r.Registry.EmitEventToOwners(ctx, "Normal", "ServiceMonitorAPIAvailable", "ServiceMonitor API is now available")
		} else {
			logger.Info("ServiceMonitor CRD is no longer available")
			r.Registry.EmitEventToOwners(ctx, "Warning", "ServiceMonitorAPIUnavailable", "ServiceMonitor API is no longer available")
		}
	}

	if available {
		logger.Info("ServiceMonitor CRD is established, reconciling all registered ServiceMonitors")
		if err := r.Registry.ReconcileAll(ctx); err != nil {
			logger.Error(err, "Failed to reconcile ServiceMonitors")
		}
	}

	return reconcile.Result{}, nil
}

// isCRDEstablished checks if a CRD is established and ready to use
func isCRDEstablished(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established {
			return condition.Status == apiextensionsv1.ConditionTrue
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager
func (r *CRDWatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		WithEventFilter(serviceMonitorCRDPredicate()).
		Complete(r)
}

// serviceMonitorCRDPredicate filters events to only process the ServiceMonitor CRD
func serviceMonitorCRDPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == ServiceMonitorCRDName
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetName() == ServiceMonitorCRDName
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == ServiceMonitorCRDName
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return e.Object.GetName() == ServiceMonitorCRDName
		},
	}
}
