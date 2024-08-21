package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/core/v1"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Ready || c.Reason == constants.Creating) && utils.OptionalBool(instance.Spec.Db.Create)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.DbComponentName, actions.DbDeploymentName, instance.Name)
	scc, err := kubernetes.GetOpenshiftPodSecurityContextRestricted(ctx, i.Client, instance.Namespace)
	if err != nil {
		i.Logger.Info("Can't resolve OpenShift scc - using default values", "Error", err.Error(), "Fallback FSGroup", "1001")
		scc = &v1.PodSecurityContext{FSGroup: utils.Pointer(int64(1001)), FSGroupChangePolicy: utils.Pointer(v1.FSGroupChangeOnRootMismatch)}
	}

	useTLS := (instance.Spec.Db.TLSCertificate.CertRef != nil) || k8sutils.IsOpenShift()
	db, err := trillianUtils.CreateTrillDb(instance, actions.DbDeploymentName, actions.RBACName, scc, labels, useTLS)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance)
	}

	// TLS certificate
	if instance.Spec.Db.TLSCertificate.CertRef != nil {
		db.Spec.Template.Spec.Volumes = append(db.Spec.Template.Spec.Volumes,
			v1.Volume{
				Name: "tls-cert",
				VolumeSource: v1.VolumeSource{
					Projected: &v1.ProjectedVolumeSource{
						Sources: []v1.VolumeProjection{
							{
								Secret: &v1.SecretProjection{
									LocalObjectReference: v1.LocalObjectReference{
										Name: instance.Spec.Db.TLSCertificate.CertRef.Name,
									},
									Items: []v1.KeyToPath{
										{
											Key:  instance.Spec.Db.TLSCertificate.CertRef.Key,
											Path: "tls.crt",
										},
									},
								},
							},
							{
								Secret: &v1.SecretProjection{
									LocalObjectReference: v1.LocalObjectReference{
										Name: instance.Spec.Db.TLSCertificate.PrivateKeyRef.Name,
									},
									Items: []v1.KeyToPath{
										{
											Key:  instance.Spec.Db.TLSCertificate.PrivateKeyRef.Key,
											Path: "tls.key",
										},
									},
								},
							},
						},
					},
				},
			})
	} else if k8sutils.IsOpenShift() {
		i.Logger.V(1).Info("TLS: Using secrets/signing-key secret")
		db.Spec.Template.Spec.Volumes = append(db.Spec.Template.Spec.Volumes,
			v1.Volume{
				Name: "tls-cert",
				VolumeSource: v1.VolumeSource{
					Projected: &v1.ProjectedVolumeSource{
						Sources: []v1.VolumeProjection{
							{
								Secret: &v1.SecretProjection{
									LocalObjectReference: v1.LocalObjectReference{
										Name: instance.Name + "-trillian-db-tls-secret",
									},
								},
							},
							{
								ConfigMap: &v1.ConfigMapProjection{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "ca-configmap",
									},
									Items: []v1.KeyToPath{
										{
											Key:  "service-ca.crt",
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			})
	} else {
		i.Logger.V(1).Info("Communication between services is insecure")
	}

	if instance.Spec.Db.TLSCertificate.CertRef != nil || k8sutils.IsOpenShift() {
		db.Spec.Template.Spec.Containers[0].VolumeMounts = append(db.Spec.Template.Spec.Containers[0].VolumeMounts,
			v1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			})
		db.Spec.Template.Spec.Containers[0].Args = append(db.Spec.Template.Spec.Containers[0].Args, "--ssl-cert", "/etc/ssl/certs/tls.crt")
		db.Spec.Template.Spec.Containers[0].Args = append(db.Spec.Template.Spec.Containers[0].Args, "--ssl-key", "/etc/ssl/certs/tls.key")
	}

	if err = controllerutil.SetControllerReference(instance, db, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for DB Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, db); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Database deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
