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
	"k8s.io/apimachinery/pkg/api/errors"
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

func (r *SecuresignReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	failResult := ctrl.Result{RequeueAfter: time.Second * 15}
	instance, err := r.ensureSecureSign(ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure tas: %w", err)
	}

	// Add finalizer for this CR
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

	// Reconcile the tracked objects
	err = r.createTrackedObjects(ctx, instance)
	if err != nil {
		log.Error(err, "failed to reconcile tas cluster")
		return failResult, err
	}
	return ctrl.Result{Requeue: false}, nil
}

// createTrackedObjects Creates a mapping from client objects to their mutating functions.
func (r *SecuresignReconciler) createTrackedObjects(
	ctx context.Context,
	instance *rhtasv1alpha1.Securesign,
) error {
	var err error
	var svc *corev1.Service

	// REKOR
	var rkn *corev1.Namespace
	var rekorNamespace = "rekor-system"
	var rrSA = "rekor-redis"
	var rrsa *corev1.ServiceAccount
	var rsSA = "rekor-server"
	var rssa *corev1.ServiceAccount
	var rtasCTSA = "trusted-artifact-signer-rekor-createtree"

	// FULCIO
	var fun *corev1.Namespace
	var fulcioNamespace = "fulcio-system"
	var fSA = "fulcio-server"
	var fsa *corev1.ServiceAccount

	// CTLOG
	var ctn *corev1.Namespace
	var ctlogNamespace = "ctlog-system"
	var ctlogSA = "ctlog"
	var ctsa *corev1.ServiceAccount
	var ctlogCTSA = "ctlog-createtree"
	var ctctsa *corev1.ServiceAccount
	var ctlogTASCCSA = "trusted-artifact-signer-ctlog-createctconfig"
	var ctctasccsa *corev1.ServiceAccount

	// TRILLIAN
	var trn *corev1.Namespace
	var trillianNamespace = "trillian-system"
	var tlsSA = "trillian-logserver"
	var tlssa *corev1.ServiceAccount
	var tlsnrSA = "trillian-logsigner"
	var tDBSA = "trillian-mysql"
	var tdbsa *corev1.ServiceAccount
	var trilllogServ = "registry.redhat.io/rhtas-tech-preview/trillian-logserver-rhel9@sha256:43bfc6b7b8ed902592f19b830103d9030b59862f959c97c376cededba2ac3a03"
	var trilllogSign = "registry.redhat.io/rhtas-tech-preview/trillian-logsigner-rhel9@sha256:fa2717c1d54400ca74cc3e9038bdf332fa834c0f5bc3215139c2d0e3579fc292"
	var trillDb = "registry.redhat.io/rhtas-tech-preview/trillian-database-rhel9@sha256:fe4758ff57a9a6943a4655b21af63fb579384dc51838af85d0089c04290b4957"
	var trillPVC *corev1.PersistentVolumeClaim
	var dbSecret *corev1.Secret

	// TUF
	var tun *corev1.Namespace
	var tufNamespace = "tuf-system"
	var tstufSA = "tuf"
	var tstufsa *corev1.ServiceAccount
	var tscj = "tuf-secret-copy-job"

	// TRUSTED ARTIFACT SIGNER
	var tascs *corev1.Namespace
	var tasNamespace = "trusted-artifact-signer-clientserver"
	var tascSA = "tas-clients"
	var tascsa *corev1.ServiceAccount

	// Create the namespaces
	if tun, err = r.ensureNamespace(ctx, instance, tufNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace tuf-system. %w", err)
	}
	if trn, err = r.ensureNamespace(ctx, instance, trillianNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace trillian-system. %w", err)
	}
	if rkn, err = r.ensureNamespace(ctx, instance, rekorNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace rekor-system. %w", err)
	}
	if fun, err = r.ensureNamespace(ctx, instance, fulcioNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace fulcio-system. %w", err)
	}
	if ctn, err = r.ensureNamespace(ctx, instance, ctlogNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace ctlog-system. %w", err)
	}
	if tascs, err = r.ensureNamespace(ctx, instance, tasNamespace); err != nil {
		return fmt.Errorf("could not ensure namespace trusted-artifact-signer-clientserver. %w", err)
	}
	// Create the service accounts
	// CTLOG
	if ctsa, err = r.ensureSA(ctx, instance, ctn.Name, ctlogSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if ctctsa, err = r.ensureSA(ctx, instance, ctn.Name, ctlogCTSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if ctctasccsa, err = r.ensureSA(ctx, instance, ctn.Name, ctlogTASCCSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// FULCIO
	if fsa, err = r.ensureSA(ctx, instance, fun.Name, fSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// REKOR
	if rrsa, err = r.ensureSA(ctx, instance, rkn.Name, rrSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if rssa, err = r.ensureSA(ctx, instance, rkn.Name, rsSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if _, err = r.ensureSA(ctx, instance, rkn.Name, rtasCTSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// TRILLIAN
	if tlssa, err = r.ensureSA(ctx, instance, trn.Name, tlsSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if _, err = r.ensureSA(ctx, instance, trn.Name, tlsnrSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if tdbsa, err = r.ensureSA(ctx, instance, trn.Name, tDBSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// TRUSTED ARTIFACT SIGNER
	if tascsa, err = r.ensureSA(ctx, instance, tascs.Name, tascSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// TUF
	if tstufsa, err = r.ensureSA(ctx, instance, tun.Name, tstufSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if _, err = r.ensureSA(ctx, instance, tun.Name, tscj); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// Create PVC
	// Trillian
	if trillPVC, err = r.ensurePVC(ctx, instance, trn.Name, "trillian-mysql"); err != nil {
		return fmt.Errorf("could not ensure pvc: %w", err)
	}

	// Create Database Secret
	// Trillian
	if dbSecret, err = r.ensureSecret(ctx, instance, trn.Name, "trillian-mysql"); err != nil {
		return fmt.Errorf("could not ensure secret: %w", err)
	}

	// Create Service
	if svc, err = r.ensureServiceCluster(ctx, instance); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// Create the deployments
	// Ctlog
	if _, err = r.ensureDeployment(ctx, instance, ctn.Name, svc.Name, ctsa.Name, "ctfiller"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureDeployment(ctx, instance, ctn.Name, svc.Name, ctctsa.Name, "ctfiller2"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureDeployment(ctx, instance, ctn.Name, svc.Name, ctctasccsa.Name, "ctfiller3"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Trillian
	if _, err = r.ensureTrillDeployment(ctx, instance, trn.Name, svc.Name, tlssa.Name, "trillian-logserver", trilllogServ, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureTrillDeployment(ctx, instance, trn.Name, svc.Name, tlssa.Name, "trillian-logsigner", trilllogSign, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureTrillDb(ctx, instance, trn.Name, svc.Name, tdbsa.Name, "trillian-mysql", trillDb, trillPVC.Name, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Rekor
	if _, err = r.ensureDeployment(ctx, instance, rkn.Name, svc.Name, rrsa.Name, "rekor-redis-filler"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureDeployment(ctx, instance, rkn.Name, svc.Name, rssa.Name, "rekor-server-filler"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Fulcio
	if _, err = r.ensureDeployment(ctx, instance, fun.Name, svc.Name, fsa.Name, "fulcio-filler"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// TUF
	if _, err = r.ensureDeployment(ctx, instance, tun.Name, svc.Name, tstufsa.Name, "tuff-filler"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Trusted Artifact Signer
	if _, err = r.ensureDeployment(ctx, instance, tascs.Name, svc.Name, tascsa.Name, "tuff-filler2"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	return nil
}

func (r *SecuresignReconciler) ensureSecureSign(
	ctx context.Context,
	req ctrl.Request,
) (*rhtasv1alpha1.Securesign, error) {
	var err error
	instance := &rhtasv1alpha1.Securesign{}
	if err = r.Get(ctx, req.NamespacedName, instance); err == nil {
		return instance, nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *SecuresignReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Securesign{}).
		Complete(r)
}
