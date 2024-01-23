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

	"github.com/securesign/operator/client"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	cliv1 "github.com/openshift/api/console/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const (
	finalizer = "tas.rhtas.redhat.com"

	ClientServerDeploymentName = "client-server"

	cosignConsoleCliName        = "cosign"
	cosignConsoleCliDescription = "cosign is a CLI tool that allows you to manage sigstore artifacts."

	rekorCliConsoleCliName        = "rekor-cli"
	rekorCliConsoleCliDescription = "rekor-cli is a CLI tool that allows you to interact with rekor server."

	gitsignConsoleCliName        = "gitsign"
	gitsignConsoleCliDescription = "gitsign is a CLI tool that allows you to digitally sign and verify git commits."
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
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=create;get;list;watch;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=console.openshift.io,resources=consoleclidownloads,verbs=create;get;list;watch;update;patch;delete

func (r *SecuresignReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	instance, err := r.ensureSecureSign(ctx, req)
	if err != nil {
		if errors.IsNotFound(err) {
			// ignore
			log.V(3).Info("securesign resource not found - it must be deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to ensure securesign resource: %w", err)
	}

	target := instance.DeepCopy()

	//Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(target, finalizer) {
		controllerutil.AddFinalizer(target, finalizer)
		err = r.Update(ctx, target)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update instance: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if instance.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(target, finalizer)
		return ctrl.Result{}, r.Update(ctx, target)
	}

	actions := []func(context.Context, *rhtasv1alpha1.Securesign) (bool, error){
		r.ensureRBAC(),
		r.ensureTrillian(),
		r.ensureFulcio(),
		r.ensureRekor(),
		r.ensureCTlog(),
		r.ensureTuf(),
		r.ensureClientServer(),
	}

	for _, a := range actions {
		update, err := a(ctx, target)
		if err != nil {
			return ctrl.Result{}, err
		}
		if update {
			err = r.Status().Update(ctx, target)
			if err != nil {
				return ctrl.Result{}, err
			}
			// requeue one by one to be always up-to-date
			return ctrl.Result{Requeue: true}, nil
		}
	}

	return ctrl.Result{}, nil
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
	return nil, fmt.Errorf("failed to get SecureSign: %w", err)
}

func (r *SecuresignReconciler) ensureRBAC() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.ServiceAccountName,
				Namespace: securesign.Namespace,
				Labels:    labels(*securesign),
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: "pull-secret",
				},
			},
		}
		ctrl.SetControllerReference(securesign, sa, r.Scheme)
		if err := r.Get(ctx, types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}, &corev1.ServiceAccount{}); err != nil {
			if errors.IsNotFound(err) {
				err = r.Create(ctx, sa)
				if err != nil {
					return false, err
				}
			}
			if err != nil {
				return false, err
			}
		}

		role := kubernetes.CreateRole(securesign.Namespace, constants.ServiceAccountName, labels(*securesign),
			[]rbac.PolicyRule{
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
			})
		ctrl.SetControllerReference(securesign, role, r.Scheme)
		if err := r.Get(ctx, types.NamespacedName{Namespace: role.Namespace, Name: role.Name}, &rbac.Role{}); err != nil {
			if errors.IsNotFound(err) {
				err = r.Create(ctx, role)
				if err != nil {
					return false, err
				}
			}
			if err != nil {
				return false, err
			}
		}

		roleBinding := kubernetes.CreateRoleBinding(securesign.Namespace, constants.ServiceAccountName, labels(*securesign),
			rbac.RoleRef{
				APIGroup: rbac.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     role.Name,
			},
			[]rbac.Subject{
				{Kind: "ServiceAccount", Name: sa.Name, Namespace: securesign.Namespace},
			})
		ctrl.SetControllerReference(securesign, roleBinding, r.Scheme)
		if err := r.Get(ctx, types.NamespacedName{Namespace: roleBinding.Namespace, Name: roleBinding.Name}, &rbac.RoleBinding{}); err != nil {
			if errors.IsNotFound(err) {
				err = r.Create(ctx, roleBinding)
				if err != nil {
					return false, err
				}
			}
			if err != nil {
				return false, err
			}
		}
		return false, nil

	}
}

func (r *SecuresignReconciler) ensureTrillian() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {
		if securesign.Status.Trillian == "" {
			instance := &rhtasv1alpha1.Trillian{}
			instance.Name = securesign.Name
			instance.Namespace = securesign.Namespace
			instance.Labels = labels(*securesign)

			instance.Spec = securesign.Spec.Trillian
			ctrl.SetControllerReference(securesign, instance, r.Scheme)
			securesign.Status.Trillian = instance.Name
			if err := r.Create(ctx, instance); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
}

func (r *SecuresignReconciler) ensureCTlog() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {
		if securesign.Status.CTlog == "" {
			instance := &rhtasv1alpha1.CTlog{}
			instance.Name = securesign.Name
			instance.Namespace = securesign.Namespace
			instance.Labels = labels(*securesign)

			instance.Spec = securesign.Spec.Ctlog
			ctrl.SetControllerReference(securesign, instance, r.Scheme)
			securesign.Status.CTlog = instance.Name
			if err := r.Create(ctx, instance); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
}

func (r *SecuresignReconciler) ensureTuf() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {
		if securesign.Status.Tuf == "" {
			instance := &rhtasv1alpha1.Tuf{}
			instance.Name = securesign.Name
			instance.Namespace = securesign.Namespace
			instance.Labels = labels(*securesign)

			instance.Spec = securesign.Spec.Tuf
			ctrl.SetControllerReference(securesign, instance, r.Scheme)
			securesign.Status.Tuf = instance.Name
			if err := r.Create(ctx, instance); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
}

func (r *SecuresignReconciler) ensureFulcio() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {
		if securesign.Status.Fulcio == "" {
			instance := &rhtasv1alpha1.Fulcio{}

			instance.Name = securesign.Name
			instance.Namespace = securesign.Namespace
			instance.Labels = labels(*securesign)
			ctrl.SetControllerReference(securesign, instance, r.Scheme)

			instance.Spec = securesign.Spec.Fulcio
			securesign.Status.Fulcio = instance.Name
			if err := r.Create(ctx, instance); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
}

func (r *SecuresignReconciler) ensureRekor() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, securesign *rhtasv1alpha1.Securesign) (bool, error) {
		if securesign.Status.Rekor == "" {
			instance := &rhtasv1alpha1.Rekor{}

			instance.Name = securesign.Name
			instance.Namespace = securesign.Namespace
			instance.Labels = labels(*securesign)
			ctrl.SetControllerReference(securesign, instance, r.Scheme)

			securesign.Status.Rekor = instance.Name
			instance.Spec = securesign.Spec.Rekor
			if err := r.Create(ctx, instance); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, nil
	}
}

func (r *SecuresignReconciler) ensureClientServer() func(context.Context, *rhtasv1alpha1.Securesign) (bool, error) {
	return func(ctx context.Context, s *rhtasv1alpha1.Securesign) (bool, error) {

		if s.Spec.ClientServer.Enabled {
			clientServerDeploymentLabels := kubernetes.FilterCommonLabels(s.Labels)
			clientServerDeploymentLabels[kubernetes.ComponentLabel] = ClientServerDeploymentName
			clientServerDeploymentLabels[kubernetes.NameLabel] = ClientServerDeploymentName
			clientServerDeployment := common.CreateClientserverDeployment(s.Namespace, ClientServerDeploymentName, clientServerDeploymentLabels)
			if err := r.Get(ctx, types.NamespacedName{Namespace: clientServerDeployment.Namespace, Name: clientServerDeployment.Name}, &apps.Deployment{}); err != nil {
				if errors.IsNotFound(err) {
					err = r.Create(ctx, clientServerDeployment)
					if err != nil {
						return false, err
					}
				}
				if err != nil {
					return false, err
				}
			}
			ctrl.SetControllerReference(s, clientServerDeployment, r.Scheme)

			clientServerService := kubernetes.CreateService(s.Namespace, ClientServerDeploymentName, 8080, clientServerDeploymentLabels)
			if err := r.Get(ctx, types.NamespacedName{Namespace: clientServerService.Namespace, Name: clientServerService.Name}, &corev1.Service{}); err != nil {
				if errors.IsNotFound(err) {
					err = r.Create(ctx, clientServerService)
					if err != nil {
						return false, err
					}
				}
				if err != nil {
					return false, err
				}
			}
			ctrl.SetControllerReference(s, clientServerService, r.Scheme)

			clientServerIngress, err := kubernetes.CreateIngress(ctx, r.Client, *clientServerService, rhtasv1alpha1.ExternalAccess{}, ClientServerDeploymentName, clientServerDeploymentLabels)
			if err != nil {
				return false, err
			}
			if err := r.Get(ctx, types.NamespacedName{Namespace: clientServerIngress.Namespace, Name: clientServerIngress.Name}, &v1.Ingress{}); err != nil {
				if errors.IsNotFound(err) {
					err = r.Create(ctx, clientServerIngress)
					if err != nil {
						return false, err
					}
				}
				if err != nil {
					return false, err
				}
			}

			protocol := "http://"
			if len(clientServerIngress.Spec.TLS) > 0 {
				protocol = "https://"
			}
			s.Status.ClientServerUrl = protocol + clientServerIngress.Spec.Rules[0].Host
			err = r.Status().Update(ctx, s)
			if err != nil {
				return false, err
			}
			ctrl.SetControllerReference(s, clientServerIngress, r.Scheme)

			if s.Spec.ClientServer.EnableOpenshiftCliDownload {
				cosignConsoleCliDownloadLabels := kubernetes.FilterCommonLabels(s.Labels)
				cosignConsoleCliDownloadLabels[kubernetes.ComponentLabel] = ClientServerDeploymentName
				cosignConsoleCliDownloadLabels[kubernetes.NameLabel] = cosignConsoleCliName
				cosignConsoleCliDownload := kubernetes.CreateConsoleCLIDownload(s.Namespace, cosignConsoleCliName, s.Status.ClientServerUrl, cosignConsoleCliDescription, cosignConsoleCliDownloadLabels)
				if err := r.Get(ctx, types.NamespacedName{Namespace: cosignConsoleCliDownload.Namespace, Name: cosignConsoleCliDownload.Name}, &cliv1.ConsoleCLIDownload{}); err != nil {
					if errors.IsNotFound(err) {
						err = r.Create(ctx, cosignConsoleCliDownload)
						if err != nil {
							return false, err
						}
					}
					if err != nil {
						return false, err
					}
				}
				ctrl.SetControllerReference(s, cosignConsoleCliDownload, r.Scheme)

				rekorCliConsoleCliDownloadLabels := kubernetes.FilterCommonLabels(s.Labels)
				rekorCliConsoleCliDownloadLabels[kubernetes.ComponentLabel] = ClientServerDeploymentName
				rekorCliConsoleCliDownloadLabels[kubernetes.NameLabel] = rekorCliConsoleCliName
				rekorCliConsoleCliDownload := kubernetes.CreateConsoleCLIDownload(s.Namespace, rekorCliConsoleCliName, s.Status.ClientServerUrl, rekorCliConsoleCliDescription, rekorCliConsoleCliDownloadLabels)
				if err := r.Get(ctx, types.NamespacedName{Namespace: rekorCliConsoleCliDownload.Namespace, Name: rekorCliConsoleCliDownload.Name}, &cliv1.ConsoleCLIDownload{}); err != nil {
					if errors.IsNotFound(err) {
						err = r.Create(ctx, rekorCliConsoleCliDownload)
						if err != nil {
							return false, err
						}
					}
					if err != nil {
						return false, err
					}
				}
				ctrl.SetControllerReference(s, rekorCliConsoleCliDownload, r.Scheme)

				gitsignConsoleCliDownloadLabels := kubernetes.FilterCommonLabels(s.Labels)
				gitsignConsoleCliDownloadLabels[kubernetes.ComponentLabel] = ClientServerDeploymentName
				gitsignConsoleCliDownloadLabels[kubernetes.NameLabel] = gitsignConsoleCliName
				gitsignConsoleCliDownload := kubernetes.CreateConsoleCLIDownload(s.Namespace, gitsignConsoleCliName, s.Status.ClientServerUrl, gitsignConsoleCliDescription, gitsignConsoleCliDownloadLabels)
				if err := r.Get(ctx, types.NamespacedName{Namespace: gitsignConsoleCliDownload.Namespace, Name: gitsignConsoleCliDownload.Name}, &cliv1.ConsoleCLIDownload{}); err != nil {
					if errors.IsNotFound(err) {
						err = r.Create(ctx, gitsignConsoleCliDownload)
						if err != nil {
							return false, err
						}
					}
					if err != nil {
						return false, err
					}
				}
				ctrl.SetControllerReference(s, gitsignConsoleCliDownload, r.Scheme)
			}
		}
		return false, nil
	}
}

func labels(instance rhtasv1alpha1.Securesign) map[string]string {
	return map[string]string{
		"app.kubernetes.io/part-of":  constants.AppName,
		"app.kubernetes.io/instance": instance.Name,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecuresignReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rhtasv1alpha1.Securesign{}).
		Complete(r)
}
