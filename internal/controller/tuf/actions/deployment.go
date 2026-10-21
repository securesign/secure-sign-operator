package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

func NewDeployAction() action.Action[*rhtasv1.Tuf] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1.Tuf) bool {
	return state.FromInstance(tuf, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {

	dp, err := traits.Apply(
		appsv1ac.Deployment(tufConstants.DeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)),
		i.createTufDeployment(instance),
		traits.PodRequirements(instance.Spec.PodRequirements, tufConstants.ContainerName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{tufConstants.ContainerName}),
			traits.GODEBUG(instance.GetAnnotations(), tufConstants.ContainerName),
			traits.TrustedCA(instance.GetTrustedCA(), tufConstants.ContainerName),
			traits.SecurityContexts(tufConstants.ContainerName),
			traits.ServiceAccount(tufConstants.RBACName),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build TUF deployment: %w", err), instance)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tufConstants.DeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func (i deployAction) createTufDeployment(instance *rhtasv1.Tuf) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		container := corev1ac.Container().
			WithName(tufConstants.ContainerName).
			WithImage(images.Registry.Get(images.HttpServer)).
			WithCommand("/bin/sh", "-c", `
sed -i 's#SecAuditLog /var/log/httpd/modsec_audit.log#SecAuditLog "|/usr/sbin/rotatelogs -n 3 /var/log/httpd/modsec_audit.log 10M"#' /etc/httpd/conf.d/mod_security.conf
grep -q rotatelogs /etc/httpd/conf.d/mod_security.conf || { echo "ERROR: failed to configure mod_security audit log rotation" >&2; exit 1; }
exec run-httpd
`).
			WithPorts(corev1ac.ContainerPort().
				WithName(tufConstants.PortName).
				WithContainerPort(tufConstants.Port).
				WithProtocol(core.ProtocolTCP)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(tufConstants.VolumeName).
				WithMountPath("/var/www/html").
				WithReadOnly(false)).
			WithLivenessProbe(corev1ac.Probe().
				WithTCPSocket(corev1ac.TCPSocketAction().
					WithPort(intstr.FromInt32(tufConstants.Port))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/root.json").
					WithPort(intstr.FromInt32(tufConstants.Port))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/root.json").
					WithPort(intstr.FromInt32(tufConstants.Port))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		podSpec := corev1ac.PodSpec().
			WithVolumes(corev1ac.Volume().
				WithName(tufConstants.VolumeName).
				WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().
					WithClaimName(instance.Status.PvcName))).
			WithContainers(container)

		dp.
			WithSpec(appsv1ac.DeploymentSpec().
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
				WithStrategy(appsv1ac.DeploymentStrategy().
					WithType(apps.RecreateDeploymentStrategyType)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(dp.Labels).
					WithSpec(podSpec)))

		return nil
	}
}
