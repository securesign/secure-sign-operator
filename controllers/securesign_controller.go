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

package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
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
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *SecuresignReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	failResult := ctrl.Result{RequeueAfter: time.Second * 15}
	instance, err := r.ensureSecureSign(ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure tas: %w", err)
	}

	//Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(instance, finalizer) {
		controllerutil.AddFinalizer(instance, finalizer)
		err = r.Update(ctx, instance)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update instance: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if instance.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(instance, finalizer)
		return ctrl.Result{}, r.Update(ctx, instance)
	}

	if err := r.ensureSa(ctx, instance); err != nil {
		return failResult, err
	}

	if err := r.ensureRole(ctx, instance); err != nil {
		return failResult, err
	}

	if err := r.ensureRoleBinding(ctx, instance); err != nil {
		return failResult, err
	}

	// Reconcile the tracked objects
	var update bool
	update, err = r.ensureTrillian(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	if update {
		r.Status().Update(ctx, instance)
		return ctrl.Result{Requeue: true}, nil
	}

	update, err = r.ensureTuf(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	if update {
		r.Status().Update(ctx, instance)
		return ctrl.Result{Requeue: true}, nil
	}

	update, err = r.ensureFulcio(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	if update {
		r.Status().Update(ctx, instance)
		return ctrl.Result{Requeue: true}, nil
	}

	update, err = r.ensureRekor(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	if update {
		r.Status().Update(ctx, instance)
		return ctrl.Result{Requeue: true}, nil
	}

	update, err = r.ensureCTlog(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	if update {
		r.Status().Update(ctx, instance)
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// createTrackedObjects Creates a mapping from client objects to their mutating functions.

func (r *SecuresignReconciler) ensureSecureSign(
	ctx context.Context,
	req ctrl.Request,
) (*rhtasv1alpha1.Securesign, error) {
	var err error
	instance := &rhtasv1alpha1.Securesign{}
	if err = r.Get(ctx, req.NamespacedName, instance); err == nil {
		return instance.DeepCopy(), nil
	}
	if errors.IsNotFound(err) {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		return nil, fmt.Errorf("Securesign Cluster resource not found, ignoring since object must be deleted: %w", err)
	}
	// Error reading the object - requeue the request.
	return nil, fmt.Errorf("failed to get SecureSign: %w", err)
}

func (r *SecuresignReconciler) ensureTrillian(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) (bool, error) {
	if securesign.Status.Trillian == "" {
		instance := &rhtasv1alpha1.Trillian{}
		instance.GenerateName = securesign.Name
		instance.Namespace = securesign.Namespace
		instance.Spec = securesign.Spec.Trillian
		ctrl.SetControllerReference(securesign, instance, r.Scheme)

		if err := r.Create(ctx, instance); err != nil {
			return false, err
		}
		securesign.Status.Trillian = instance.Name
		return true, nil
	}
	return false, nil
}

func (r *SecuresignReconciler) ensureCTlog(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) (bool, error) {
	if securesign.Status.CTlog == "" {
		instance := &rhtasv1alpha1.CTlog{}
		instance.GenerateName = securesign.Name
		instance.Namespace = securesign.Namespace
		instance.Spec = securesign.Spec.Ctlog
		ctrl.SetControllerReference(securesign, instance, r.Scheme)

		if err := r.Create(ctx, instance); err != nil {
			return false, err
		}
		securesign.Status.CTlog = instance.Name
		return true, nil
	}
	return false, nil
}

func (r *SecuresignReconciler) ensureTuf(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) (bool, error) {
	if securesign.Status.Tuf == "" {
		instance := &rhtasv1alpha1.Tuf{}
		instance.GenerateName = securesign.Name
		instance.Namespace = securesign.Namespace
		instance.Spec = securesign.Spec.Tuf
		ctrl.SetControllerReference(securesign, instance, r.Scheme)

		if err := r.Create(ctx, instance); err != nil {
			return false, err
		}
		securesign.Status.Tuf = instance.Name
		return true, nil
	}
	return false, nil
}

func (r *SecuresignReconciler) ensureFulcio(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) (bool, error) {
	if securesign.Status.Fulcio == "" {
		instance := &rhtasv1alpha1.Fulcio{}

		instance.GenerateName = securesign.Name
		instance.Namespace = securesign.Namespace
		ctrl.SetControllerReference(securesign, instance, r.Scheme)

		instance.Spec = securesign.Spec.Fulcio
		if err := r.Create(ctx, instance); err != nil {
			return false, err
		}
		securesign.Status.Fulcio = instance.Name
		return true, nil
	}
	return false, nil
}

func (r *SecuresignReconciler) ensureRekor(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) (bool, error) {
	if securesign.Status.Rekor == "" {
		instance := &rhtasv1alpha1.Rekor{}

		instance.GenerateName = securesign.Name
		instance.Namespace = securesign.Namespace
		ctrl.SetControllerReference(securesign, instance, r.Scheme)

		instance.Spec = securesign.Spec.Rekor
		if err := r.Create(ctx, instance); err != nil {
			return false, err
		}
		securesign.Status.Rekor = instance.Name
		return true, nil
	}
	return false, nil
}

func (r *SecuresignReconciler) ensureSa(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sigstore-sa",
			Namespace: securesign.Namespace,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "pull-secret",
			},
		},
	}
	// Check if this service account already exists else create it in the namespace
	err := r.Get(ctx, client.ObjectKey{Name: sa.Name, Namespace: securesign.Namespace}, sa)
	// If the SA doesn't exist, create it but if it does, do nothing
	if err != nil {
		err = r.Create(ctx, sa)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SecuresignReconciler) ensureRole(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) error {
	name := "securesign"
	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: securesign.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "securesign",
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create", "get", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "get", "update"},
			},
		},
	}

	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: securesign.Namespace}, role)
	if err != nil {
		err = r.Create(ctx, role)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *SecuresignReconciler) ensureRoleBinding(
	ctx context.Context,
	securesign *rhtasv1alpha1.Securesign,
) error {

	name := "securesign"
	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: securesign.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     name,
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},

		// todo: remove hardcoded names
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "sigstore-sa",
				Namespace: securesign.Namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "securesign",
		},
	}

	// If the bindingName is tuf-secret-copy-job* then change the kind of Role to clusterrole
	// The Namespace for the serviceAccount will be tuf-system

	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: securesign.Namespace}, roleBinding)
	if err != nil {
		err = r.Create(ctx, roleBinding)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecuresignReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Securesign{}).
		Complete(r)
}
