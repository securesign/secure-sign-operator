package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		updated bool
		err     error
	)

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	dp, err := futils.CreateDeployment(instance, DeploymentName, RBACName, labels)
	if err != nil {
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create server Deployment: %w", err), instance)
		}
	}
	// TLS certificate
	signingKeySecret, _ := k8sutils.GetSecret(i.Client, "openshift-service-ca", "signing-key")
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
	} else if signingKeySecret != nil {
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
										Name: instance.Name + "-tls-secret",
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

	if instance.Spec.TLSCertificate.CertRef != nil || signingKeySecret != nil {
		dp.Spec.Template.Spec.Containers[0].VolumeMounts = append(dp.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			})
		dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--grpc-tls-certificate", "/etc/ssl/certs/tls.crt")
		dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--grpc-tls-key", "/etc/ssl/certs/tls.key")
		dp.Spec.Template.Spec.Containers[0].Args = append(dp.Spec.Template.Spec.Containers[0].Args, "--tls-ca-cert", "/etc/ssl/certs/ca.crt")
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Deployment created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
