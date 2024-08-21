package logsigner

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
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

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions.LogSignerComponentName, actions.LogsignerDeploymentName, instance.Name)
	signer, err := trillianUtils.CreateTrillDeployment(instance, constants.TrillianLogSignerImage, actions.LogsignerDeploymentName, actions.RBACName, labels)
	if err != nil {
		return i.Failed(err)
	}

	signer.Spec.Template.Spec.Containers[0].Args = append(signer.Spec.Template.Spec.Containers[0].Args, "--force_master=true")
	err = utils.SetTrustedCA(&signer.Spec.Template, utils.TrustedCAAnnotationToReference(instance.Annotations))
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian LogSigner: %w", err), instance)
	}

	if err = controllerutil.SetControllerReference(instance, signer, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for LogSigner deployment: %w", err))
	}

	// TLS certificate
	if instance.Spec.TrillianSigner.TLSCertificate.CertRef != nil {
		signer.Spec.Template.Spec.Volumes = append(signer.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianSigner.TLSCertificate.CertRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TrillianSigner.TLSCertificate.CertRef.Key,
											Path: "tls.crt",
										},
									},
								},
							},
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianSigner.TLSCertificate.PrivateKeyRef.Name,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  instance.Spec.TrillianSigner.TLSCertificate.PrivateKeyRef.Key,
											Path: "tls.key",
										},
									},
								},
							},
							{
								ConfigMap: &corev1.ConfigMapProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Spec.TrillianSigner.TLSCertificate.CACertRef.Name,
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
		i.Logger.V(1).Info("TLS: Using secrets/signing-key secret")
		signer.Spec.Template.Spec.Volumes = append(signer.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{
							{
								Secret: &corev1.SecretProjection{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: instance.Name + "-trillian-log-server-tls-secret",
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

	if instance.Spec.TrillianSigner.TLSCertificate.CertRef != nil || k8sutils.IsOpenShift() {
		signer.Spec.Template.Spec.Containers[0].VolumeMounts = append(signer.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			})
		signer.Spec.Template.Spec.Containers[0].Args = append(signer.Spec.Template.Spec.Containers[0].Args, "--mysql_tls_ca", "/etc/ssl/certs/ca.crt")
		signer.Spec.Template.Spec.Containers[0].Args = append(signer.Spec.Template.Spec.Containers[0].Args, "--mysql_server_name", "$(MYSQL_HOSTNAME)."+instance.Namespace+".svc")
	}

	if updated, err = i.Ensure(ctx, signer); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
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
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian LogSigner deployment: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
