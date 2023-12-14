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
	// ClusterRole
	var copyRole = "tas-secret-copy-job-role"

	// REKOR
	var rkn *corev1.Namespace
	var rekorNamespace = "rekor-system"
	var rrSA = "rekor-redis"
	var rrsa *corev1.ServiceAccount
	var rsSA = "rekor-server"
	var rssa *corev1.ServiceAccount
	var rtasCTSA = "trusted-artifact-signer-rekor-createtree"
	var rtasctsa *corev1.ServiceAccount

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
	//var ctlogCTSA = "ctlog-createtree"
	//var ctctsa *corev1.ServiceAccount
	var ctlogTASCCSA = "trusted-artifact-signer-ctlog-createctconfig"
	var ctctasccsa *corev1.ServiceAccount

	// TRILLIAN
	var trn *corev1.Namespace
	var trillianNamespace = "trillian-system"
	var tlsSA = "trillian-logserver"
	var tlssa *corev1.ServiceAccount
	var tlsnrSA = "trillian-logsigner"
	var tlsnrsa *corev1.ServiceAccount
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
	var tscjSA = "tuf-secret-copy-job"
	var tscjsa *corev1.ServiceAccount

	// TRUSTED ARTIFACT SIGNER
	var tascs *corev1.Namespace
	var tasNamespace = "trusted-artifact-signer-clientserver"
	var tascSA = "tas-clients"
	var tascsa *corev1.ServiceAccount

	// Create clusterrole
	if _, err = r.ensureClusterRole(ctx, instance, copyRole); err != nil {
		return fmt.Errorf("could not ensure clusterrole: %w", err)
	}
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
	// Create roles
	// CTLOG
	if _, err = r.ensureRole(ctx, instance, ctn.Name, "ctlog-cm-operator", "ctlog"); err != nil {
		return fmt.Errorf("could not ensure role: %w", err)
	}
	if _, err = r.ensureRole(ctx, instance, ctn.Name, "ctlog-secret-operator", "ctlog"); err != nil {
		return fmt.Errorf("could not ensure role: %w", err)
	}
	// REKOR
	if _, err = r.ensureRole(ctx, instance, rkn.Name, "rekor-cm-operator", "rekor"); err != nil {
		return fmt.Errorf("could not ensure role: %w", err)
	}
	// TUF
	if _, err = r.ensureRole(ctx, instance, tun.Name, "tuf", "tuf"); err != nil {
		return fmt.Errorf("could not ensure role: %w", err)
	}
	// Create the service accounts
	// CTLOG
	if ctsa, err = r.ensureSA(ctx, instance, ctn.Name, ctlogSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	//if ctctsa, err = r.ensureSA(ctx, instance, ctn.Name, ctlogCTSA); err != nil {
	//	return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	//}
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
	if rtasctsa, err = r.ensureSA(ctx, instance, rkn.Name, rtasCTSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// TRILLIAN
	if tlssa, err = r.ensureSA(ctx, instance, trn.Name, tlsSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	if tlsnrsa, err = r.ensureSA(ctx, instance, trn.Name, tlsnrSA); err != nil {
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
	if tscjsa, err = r.ensureSA(ctx, instance, tun.Name, tscjSA); err != nil {
		return fmt.Errorf("retrieved error while ensuring SA: %w", err)
	}
	// Create the rolebindings
	// CTLOG
	if _, err = r.ensureRoleBinding(ctx, instance, ctn.Name, "ctlog-cm-operator", "ctlog-cm-operator", ctsa.Name, "ctlog", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	if _, err = r.ensureRoleBinding(ctx, instance, ctn.Name, "ctlog-secret-operator", "ctlog-secret-operator", ctctasccsa.Name, "ctlog", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	// REKOR
	if _, err = r.ensureRoleBinding(ctx, instance, rkn.Name, "rekor-cm-operator", "rekor-cm-operator", rtasctsa.Name, "rekor", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	// TUF
	if _, err = r.ensureRoleBinding(ctx, instance, tun.Name, "tuf", "tuf", tstufsa.Name, "tuf", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	if _, err = r.ensureRoleBinding(ctx, instance, rkn.Name, "tuf-secret-copy-job-rekor-binding", "tas-secret-copy-job-role", tscjsa.Name, "tuf", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	if _, err = r.ensureRoleBinding(ctx, instance, ctn.Name, "tuf-secret-copy-job-ctlog-binding", "tas-secret-copy-job-role", tscjsa.Name, "tuf", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	if _, err = r.ensureRoleBinding(ctx, instance, fun.Name, "tuf-secret-copy-job-fulcio-binding", "tas-secret-copy-job-role", tscjsa.Name, "tuf", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	if _, err = r.ensureRoleBinding(ctx, instance, tun.Name, "tuf-secret-copy-job-binding", "tas-secret-copy-job-role", tscjsa.Name, "tuf", tun.Name); err != nil {
		return fmt.Errorf("could not ensure rolebinding: %w", err)
	}
	// Create Job
	if _, err = r.ensureTufCopyJob(ctx, instance, tun.Name, tscjsa.Name, "tuf-secret-copy-job", rkn.Name, fun.Name, ctn.Name); err != nil {
		return fmt.Errorf("could not ensure job: %w", err)
	}
	if _, err = r.ensureCTRekorJob(ctx, instance, rkn.Name, rtasctsa.Name, "rekor", "trusted-artifact-signer-rekor-createtree", trn.Name); err != nil {
		return fmt.Errorf("could not ensure job: %w", err)
	}
	//if _, err = r.ensureCreateDbJob(ctx, instance, tun.Name, tlssa.Name, "trillian", "trusted-artifact-signer-trillian-createdb", dbSecret.Name); err != nil {
	//	return fmt.Errorf("could not ensure job: %w", err)
	//}
	// Create PVC
	// Trillian
	if trillPVC, err = r.ensurePVC(ctx, instance, trn.Name, "trillian-mysql"); err != nil {
		return fmt.Errorf("could not ensure pvc: %w", err)
	}
	if _, err = r.ensurePVC(ctx, instance, rkn.Name, "rekor-server"); err != nil {
		return fmt.Errorf("could not ensure pvc: %w", err)
	}
	// Create ConfigMap
	// Rekor
	if _, err = r.ensureConfigMap(ctx, instance, rkn.Name, "rekor-config", "rekor"); err != nil {
		return fmt.Errorf("could not ensure configmap: %w", err)
	}
	if _, err = r.ensureConfigMap(ctx, instance, rkn.Name, "rekor-sharding-config", "rekor"); err != nil {
		return fmt.Errorf("could not ensure configmap: %w", err)
	}
	// Ctlog
	if _, err = r.ensureConfigMap(ctx, instance, ctn.Name, "ctlog-config", "ctlog"); err != nil {
		return fmt.Errorf("could not ensure configmap: %w", err)
	}
	// Fulcio
	if _, err = r.ensureOIDCConfigMap(ctx, instance, fun.Name, "fulcio-server-config", "fulcio"); err != nil {
		return fmt.Errorf("could not ensure configmap: %w", err)
	}

	// Create Secret
	// Trillian
	if dbSecret, err = r.ensureDBSecret(ctx, instance, trn.Name, "trillian-mysql"); err != nil {
		return fmt.Errorf("could not ensure secret: %w", err)
	}
	// Fulcio
	//	if _, err = r.ensureFulcioSecret(ctx, instance, fun.Name, "fulcio-secret-rh"); err != nil {
	//		return fmt.Errorf("could not ensure secret: %w", err)
	//	}
	// Rekor
	if _, err = r.ensureRekorSecret(ctx, instance, rkn.Name, "rekor-private-key"); err != nil {
		return fmt.Errorf("could not ensure secret: %w", err)
	}

	// Create Service
	// Trillian
	if _, err = r.ensureService(ctx, instance, trn.Name, "trillian-mysql", "mysql", "trillian", 3306); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	if _, err = r.ensureService(ctx, instance, trn.Name, "trillian-logserver", "trillian-logserver", "trillian", 8090); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	if _, err = r.ensureService(ctx, instance, trn.Name, "trillian-logsigner", "trillian-logsigner", "trillian", 8091); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// Ctlog
	if _, err = r.ensureService(ctx, instance, ctn.Name, "ctlog", "ctlog", "ctlog", 6963); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// Rekor
	if _, err = r.ensureService(ctx, instance, rkn.Name, "rekor-server", "rekor-server", "rekor", 2112); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	if _, err = r.ensureService(ctx, instance, rkn.Name, "rekor-redis", "redis", "rekor", 6379); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// Fulcio
	if _, err = r.ensureService(ctx, instance, fun.Name, "fulcio-server", "fulcio-server", "fulcio", 2112); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// TUF
	if _, err = r.ensureService(ctx, instance, tun.Name, "tuf", "tuf", "tuf", 80); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}
	// Trusted Artifact Signer
	if _, err = r.ensureService(ctx, instance, tascs.Name, "tas-clients", "tas-clients", "tas", 8080); err != nil {
		return fmt.Errorf("could not ensure service: %w", err)
	}

	// Create the deployments
	// Ctlog
	if _, err = r.ensurectDeployment(ctx, instance, ctn.Name, "ctlog", ctsa.Name, "ctlog"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}

	// Trillian
	if _, err = r.ensureTrillDeployment(ctx, instance, trn.Name, tlssa.Name, "trillian-logserver", trilllogServ, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureTrillDeployment(ctx, instance, trn.Name, tlsnrsa.Name, "trillian-logsigner", trilllogSign, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureTrillDb(ctx, instance, trn.Name, tdbsa.Name, "trillian-mysql", trillDb, trillPVC.Name, dbSecret.Name); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Rekor
	if _, err = r.ensureRekorDeployment(ctx, instance, rkn.Name, rssa.Name, "rekor-server"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	if _, err = r.ensureRedisDeployment(ctx, instance, rkn.Name, rrsa.Name, "rekor-redis"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// Fulcio
	if _, err = r.ensureFulDeployment(ctx, instance, fun.Name, "fulcio-server", fsa.Name, "fulcio", "server"); err != nil {
		return fmt.Errorf("could not ensure deployment: %w", err)
	}
	// TUF
	// ensure the secret tuf-secrets exists before creating the deployment log but dont fail and move on
	if err = r.Get(ctx, client.ObjectKey{Name: "tuf-secrets", Namespace: tun.Name}, &corev1.Secret{}); err == nil {
		if _, err = r.ensureTufDeployment(ctx, instance, tun.Name, tstufsa.Name, "tuf"); err != nil {
			return fmt.Errorf("could not ensure deployment: %w", err)
		}
	}
	// Trusted Artifact Signer
	if _, err = r.ensureTasDeployment(ctx, instance, tascs.Name, tascsa.Name, "tas-clients"); err != nil {
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
