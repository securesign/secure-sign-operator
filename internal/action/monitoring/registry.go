package monitoring

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceMonitorSpec holds the specification for creating a ServiceMonitor
type ServiceMonitorSpec struct {
	// Owner is the object that owns this ServiceMonitor
	OwnerKey types.NamespacedName
	// OwnerGVK is the GroupVersionKind of the owner
	OwnerGVK schema.GroupVersionKind
	// Namespace where the ServiceMonitor should be created
	Namespace string
	// Name of the ServiceMonitor
	Name string
	// EnsureFuncs are the functions to apply to the ServiceMonitor
	EnsureFuncs []func(*unstructured.Unstructured) error
}

// ownerKey uniquely identifies an owner by namespace, name, and GVK
type ownerKey struct {
	types.NamespacedName
	GVK schema.GroupVersionKind
}

// ServiceMonitorRegistry manages registered ServiceMonitor specifications
type ServiceMonitorRegistry struct {
	mutex          sync.RWMutex
	specs          map[types.NamespacedName]*ServiceMonitorSpec
	notifiedOwners map[ownerKey]*ServiceMonitorSpec
	client         client.Client
	recorder       record.EventRecorder
	logger         logr.Logger
	apiAvailable   bool
}

// NewRegistry creates a new ServiceMonitorRegistry instance
func NewRegistry(client client.Client, recorder record.EventRecorder, logger logr.Logger) *ServiceMonitorRegistry {
	return &ServiceMonitorRegistry{
		specs:          make(map[types.NamespacedName]*ServiceMonitorSpec),
		notifiedOwners: make(map[ownerKey]*ServiceMonitorSpec),
		client:         client,
		recorder:       recorder,
		logger:         logger,
	}
}

// Register adds or updates a ServiceMonitor specification in the registry
func (r *ServiceMonitorRegistry) Register(ctx context.Context, spec *ServiceMonitorSpec, owner client.Object) {
	isNewOwner, apiAvailable := r.registerSpec(spec)

	if isNewOwner {
		if apiAvailable {
			r.recorder.Event(owner, "Normal", "ServiceMonitorAPIAvailable", "ServiceMonitor API is available")
		} else {
			r.recorder.Event(owner, "Warning", "ServiceMonitorAPIUnavailable", "ServiceMonitor API is not available")
		}
	}
}

// registerSpec adds or updates a ServiceMonitor specification in the registry (internal, holds lock)
func (r *ServiceMonitorRegistry) registerSpec(spec *ServiceMonitorSpec) (isNewOwner bool, apiAvailable bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	key := types.NamespacedName{Namespace: spec.Namespace, Name: spec.Name}
	existing, exists := r.specs[key]

	newOwnerKey := ownerKey{NamespacedName: spec.OwnerKey, GVK: spec.OwnerGVK}
	isNewOwner = r.notifiedOwners[newOwnerKey] == nil

	r.specs[key] = spec

	if !exists {
		r.logger.Info("Registered ServiceMonitor spec", "owner", spec.OwnerKey, "ownerGVK", spec.OwnerGVK, "namespace", spec.Namespace, "name", spec.Name)
	} else {
		ownerChanged := existing.OwnerGVK.String() != spec.OwnerGVK.String() ||
			existing.OwnerKey != spec.OwnerKey
		changed := ownerChanged || len(existing.EnsureFuncs) != len(spec.EnsureFuncs)

		if ownerChanged {
			oldKey := ownerKey{NamespacedName: existing.OwnerKey, GVK: existing.OwnerGVK}
			delete(r.notifiedOwners, oldKey)
			r.logger.Info("Removed old owner from notification tracking", "oldOwner", existing.OwnerKey, "oldOwnerGVK", existing.OwnerGVK)

			isNewOwner = true
		}

		if changed {
			r.logger.Info("Updated ServiceMonitor spec", "owner", spec.OwnerKey, "ownerGVK", spec.OwnerGVK, "namespace", spec.Namespace, "name", spec.Name)
		}
	}

	if isNewOwner {
		r.notifiedOwners[newOwnerKey] = spec
	}

	apiAvailable = r.apiAvailable
	return isNewOwner, apiAvailable
}

// cleanupOwner removes all specs for an owner and clears it from notifiedOwners
func (r *ServiceMonitorRegistry) cleanupOwner(spec *ServiceMonitorSpec) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	specKey := types.NamespacedName{Namespace: spec.Namespace, Name: spec.Name}
	delete(r.specs, specKey)
	r.logger.V(1).Info("Removed ServiceMonitor spec for deleted owner", "owner", spec.OwnerKey, "ownerGVK", spec.OwnerGVK, "namespace", spec.Namespace, "name", spec.Name)

	ownerKey := ownerKey{NamespacedName: spec.OwnerKey, GVK: spec.OwnerGVK}
	delete(r.notifiedOwners, ownerKey)
	r.logger.V(1).Info("Cleared notification tracking for deleted owner", "owner", spec.OwnerKey, "ownerGVK", spec.OwnerGVK)
}

// GetAll returns all registered ServiceMonitor specifications
func (r *ServiceMonitorRegistry) GetAll() []*ServiceMonitorSpec {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	specs := make([]*ServiceMonitorSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	return specs
}

// SetAPIAvailable sets whether the ServiceMonitor API is available
func (r *ServiceMonitorRegistry) SetAPIAvailable(available bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.apiAvailable = available
	r.logger.Info("ServiceMonitor API availability updated", "available", available)
}

// IsAPIAvailable returns whether we should attempt to reconcile ServiceMonitors
func (r *ServiceMonitorRegistry) IsAPIAvailable() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.apiAvailable
}

// EmitEventToOwners emits an event to all unique owner objects that have registered ServiceMonitors
func (r *ServiceMonitorRegistry) EmitEventToOwners(ctx context.Context, eventType, reason, message string) {
	r.mutex.RLock()
	specsToNotify := make([]*ServiceMonitorSpec, 0, len(r.notifiedOwners))
	for _, spec := range r.notifiedOwners {
		specsToNotify = append(specsToNotify, spec)
	}
	r.mutex.RUnlock()

	for _, spec := range specsToNotify {
		r.emitEventToOwner(ctx, spec, eventType, reason, message)
	}
}

// ReconcileAll creates or updates all registered ServiceMonitors
func (r *ServiceMonitorRegistry) ReconcileAll(ctx context.Context) error {
	specs := r.GetAll()

	r.logger.Info("Reconciling all ServiceMonitors", "count", len(specs))

	if !r.IsAPIAvailable() {
		return fmt.Errorf("ServiceMonitor API not available")
	}

	var errs []error
	for _, spec := range specs {
		if err := r.ReconcileOne(ctx, spec); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to reconcile %d ServiceMonitors: %v", len(errs), errs)
	}

	return nil
}

// getOwnerObject retrieves the owner object for a ServiceMonitor spec
func (r *ServiceMonitorRegistry) getOwnerObject(ctx context.Context, spec *ServiceMonitorSpec) (client.Object, error) {
	owner := &unstructured.Unstructured{}
	owner.SetGroupVersionKind(spec.OwnerGVK)

	if err := r.client.Get(ctx, spec.OwnerKey, owner); err != nil {
		if client.IgnoreNotFound(err) == nil {
			r.logger.Info("Owner not found, cleaning up registry", "owner", spec.OwnerKey)
			r.cleanupOwner(spec)
		}
		return nil, fmt.Errorf("failed to get owner object: %w", err)
	}

	return owner, nil
}

// emitEventToOwner emits an event to the owner object of a ServiceMonitor spec
func (r *ServiceMonitorRegistry) emitEventToOwner(ctx context.Context, spec *ServiceMonitorSpec, eventType, reason, message string) {
	owner, err := r.getOwnerObject(ctx, spec)
	if err == nil {
		r.recorder.Event(owner, eventType, reason, message)
	} else {
		r.logger.Info("Failed to get owner for event emission", "owner", spec.OwnerKey, "ownerGVK", spec.OwnerGVK, "error", err.Error())
	}
}

// ReconcileOne creates or updates a single ServiceMonitor
func (r *ServiceMonitorRegistry) ReconcileOne(ctx context.Context, spec *ServiceMonitorSpec) error {
	if !r.IsAPIAvailable() {
		return fmt.Errorf("ServiceMonitor API not available")
	}

	serviceMonitor := kubernetes.CreateServiceMonitor(spec.Namespace, spec.Name)

	result, err := kubernetes.CreateOrUpdate(ctx, r.client, serviceMonitor, spec.EnsureFuncs...)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile ServiceMonitor", "namespace", spec.Namespace, "name", spec.Name)
		r.emitEventToOwner(ctx, spec, "Warning", "ServiceMonitorReconcileFailed",
			fmt.Sprintf("Failed to reconcile ServiceMonitor %s/%s: %v", spec.Namespace, spec.Name, err))
		return fmt.Errorf("failed to reconcile ServiceMonitor: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		r.logger.Info("Reconciled ServiceMonitor", "namespace", spec.Namespace, "name", spec.Name, "operation", result)
		r.emitEventToOwner(ctx, spec, "Normal", "ServiceMonitorReconciled",
			fmt.Sprintf("ServiceMonitor %s/%s %s", spec.Namespace, spec.Name, result))
	}

	return nil
}
