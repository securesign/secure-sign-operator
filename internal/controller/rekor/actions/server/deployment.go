package server

import (
	"context"
	"fmt"

	cutils "github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/core/v1"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/rekor/utils"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	insCopy := instance.DeepCopy()
	if insCopy.Spec.Trillian.Address == "" {
		insCopy.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", actions2.LogserverDeploymentName, instance.Namespace)
	}
	i.Logger.V(1).Info("trillian logserver", "address", insCopy.Spec.Trillian.Address)
	dp, err := utils.CreateRekorDeployment(insCopy, actions.ServerDeploymentName, actions.RBACName, labels)
	if err == nil {
		err = cutils.SetTrustedCA(&dp.Spec.Template, cutils.TrustedCAAnnotationToReference(instance.Annotations))
	}

	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}

	// TLS certificate
	if instance.Spec.TLSCertificate.CACertRef != nil {
		dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								ConfigMap: &corev1.ConfigMapProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TLSCertificate.CACertRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "ca.crt", // User should use this key.
											Path: "ca.crt",
										},
									},
								},
							},
						},
					},
				},
			})
	} else if k8sutils.IsOpenShift() {
		dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								ConfigMap: &corev1.ConfigMapProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "ca-configmap",
									},
									Items: []corev1.KeyToPath{
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

	if instance.Spec.TLSCertificate.CACertRef != nil || k8sutils.IsOpenShift() {
		dp.Spec.Template.Spec.Containers[0].VolumeMounts = append(dp.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/var/run/secrets/tas",
				ReadOnly:  true,
			})
		dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--trillian_log_server.tls_ca_cert", "/var/run/secrets/tas/ca.crt")
	}

	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Rekor server: %w", err), instance)
	}

	if updated {
		// Regenerate secret with public key
		instance.Status.PublicKeyRef = nil

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "DeploymentUpdated", "Deployment updated: %s", instance.Name)
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
