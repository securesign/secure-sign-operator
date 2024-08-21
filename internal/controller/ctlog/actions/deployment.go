package actions

import (
	"context"
	"fmt"

	cutils "github.com/securesign/operator/internal/controller/common/utils"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		updated bool
		err     error
	)

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	switch {
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	dp, err := utils.CreateDeployment(instance, DeploymentName, RBACName, labels, ServerTargetPort, MetricsPort)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
	}
	err = cutils.SetTrustedCA(&dp.Spec.Template, cutils.TrustedCAAnnotationToReference(instance.Annotations))
	if err != nil {
		return i.Failed(err)
	}

	// TLS certificate
	if instance.Spec.TLSCertificate.CertRef != nil && instance.Spec.TLSCertificate.CACertRef != nil {
		dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TLSCertificate.CertRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TLSCertificate.CertRef.Key,
											Path: "tls.crt",
										},
									},
								},
							},
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TLSCertificate.PrivateKeyRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TLSCertificate.PrivateKeyRef.Key,
											Path: "tls.key",
										},
									},
								},
							},
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
	} else if kubernetes.IsOpenShift() {
		i.Logger.V(1).Info("TLS: Using secrets/signing-key secret")
		dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + "-ctlog-tls-secret",
									},
								},
							},
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

	if instance.Spec.TLSCertificate.CertRef != nil && instance.Spec.TLSCertificate.CACertRef != nil || kubernetes.IsOpenShift() {
		dp.Spec.Template.Spec.Containers[0].VolumeMounts = append(dp.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/var/run/secrets/tas",
				ReadOnly:  true,
			})
		// dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--tls_certificate", "/var/run/secrets/tas/tls.crt")
		// dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--tls_key", "/var/run/secrets/tas/tls.key")
		dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--trillian_tls_ca_cert_file", "/var/run/secrets/tas/ca.crt")
	}

	if err = controllerutil.SetControllerReference(instance, dp, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, dp); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create CTlog: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Service created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
