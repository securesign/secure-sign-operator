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

package securesign

import (
	"context"

	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/operator-framework/operator-lib/predicate"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/securesign/actions"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	finalizer = "tas.rhtas.redhat.com"
)

// SecuresignReconciler reconciles a Securesign object
type SecuresignReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=securesigns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=securesigns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rhtas.redhat.com,resources=securesigns/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.openshift.io,resources=ingresscontrollers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/finalizers,verbs=update
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheuses/api,verbs=get;create;update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;get;list;watch;update;patch
//+kubebuilder:rbac:groups="operator.openshift.io",resources=consoles,verbs=get;list

// TODO: rework Securesign controller to watch resources
func (r *SecuresignReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var instance rhtasv1alpha1.Securesign
	log := ctrllog.FromContext(ctx)

	if err := r.Client.Get(ctx, req.NamespacedName, &instance); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the namespace
	var namespace v12.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: req.Namespace}, &namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	target := instance.DeepCopy()

	//Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(target, finalizer) {
		controllerutil.AddFinalizer(target, finalizer)
		return ctrl.Result{}, r.Update(ctx, target)
	}

	if instance.DeletionTimestamp != nil {
		labels := constants.LabelsFor(actions.SegmentBackupJobName, actions.SegmentBackupCronJobName, instance.Name)
		labels["app.kubernetes.io/instance-namespace"] = instance.Namespace
		if err := r.Client.DeleteAllOf(ctx, &v1.ClusterRoleBinding{}, client.MatchingLabels(labels)); err != nil {
			log.Error(err, "problem with removing clusterRoleBinding resource")
		}
		if err := r.Client.DeleteAllOf(ctx, &v1.ClusterRole{}, client.MatchingLabels(labels)); err != nil {
			log.Error(err, "problem with removing ClusterRole resource")
		}
		if err := r.Client.DeleteAllOf(ctx, &v1.Role{}, client.InNamespace(actions.OpenshiftMonitoringNS), client.MatchingLabels(labels)); err != nil {
			log.Error(err, "problem with removing Role resource in %s", actions.OpenshiftMonitoringNS)
		}
		if err := r.Client.DeleteAllOf(ctx, &v1.RoleBinding{}, client.InNamespace(actions.OpenshiftMonitoringNS), client.MatchingLabels(labels)); err != nil {
			log.Error(err, "problem with removing RoleBinding resource in %s", actions.OpenshiftMonitoringNS)
		}

		controllerutil.RemoveFinalizer(target, finalizer)
		return ctrl.Result{}, r.Update(ctx, target)
	}

	// Check if the namespace is marked for deletion
	if !namespace.DeletionTimestamp.IsZero() {
		log.Info("namespace is marked for deletion, stopping reconciliation", "namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	acs := []action.Action[*rhtasv1alpha1.Securesign]{
		actions.NewInitializeStatusAction(),
		actions.NewTrillianAction(),
		actions.NewFulcioAction(),
		actions.NewRekorAction(),
		actions.NewCtlogAction(),
		actions.NewTufAction(),
		actions.NewTsaAction(),
		actions.NewRBACAction(),
		actions.NewSegmentBackupJobAction(),
		actions.NewSegmentBackupCronJobAction(),
		actions.NewUpdateStatusAction(),
	}

	for _, a := range acs {
		a.InjectClient(r.Client)
		a.InjectLogger(log.WithName(a.Name()))

		if a.CanHandle(ctx, target) {
			result := a.Handle(ctx, target)
			if result != nil {
				return result.Result, result.Err
			}
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecuresignReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter out with the pause annotation.
	pause, err := predicate.NewPause(annotations.PausedReconciliation)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(pause).
		For(&rhtasv1alpha1.Securesign{}).
		Owns(&rhtasv1alpha1.Fulcio{}).
		Owns(&rhtasv1alpha1.Rekor{}).
		Owns(&rhtasv1alpha1.Tuf{}).
		Owns(&rhtasv1alpha1.Trillian{}).
		Owns(&rhtasv1alpha1.CTlog{}).
		Owns(&rhtasv1alpha1.TimestampAuthority{}).
		Complete(r)
}
