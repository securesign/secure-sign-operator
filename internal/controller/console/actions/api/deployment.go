package api

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const waitForTUFCommand = `curl -fL --no-progress-meter --output /dev/null --retry 50 --retry-delay 5 --retry-all-errors "${TUF_URL}/root.json" && echo 'Successfully connected to TUF repository'`

func NewDeployAction() action.Action[*rhtasv1.Console] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "api deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	tufURL, err := serviceresolver.ResolveInternalServiceUrl(ctx, i.Client, instance.Spec.Api.Tuf, instance.Namespace, &rhtasv1.Tuf{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving TUF URL: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for TUF service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.ApiDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(actions.ApiComponentName, actions.ApiDeploymentName, instance.Name)),
		ensureDeployment(tufURL),
		traits.PodRequirements(instance.Spec.Api.PodRequirements, actions.ApiDeploymentName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{actions.ApiDeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), actions.ApiDeploymentName),
			traits.TrustedCA(instance.GetTrustedCA(), actions.ApiDeploymentName),
			traits.SecurityContexts(actions.ApiDeploymentName),
			traits.ServiceAccount(actions.RBACApiName),
			traits.Optional(
				statusTLS(instance).CertRef != nil,
				ensureTLS(statusTLS(instance)),
			),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build console api deployment: %w", err), instance, metav1.Condition{
			Type: actions.ApiCondition, Status: metav1.ConditionFalse,
			Reason: state.Failure.String(), Message: err.Error(),
		})
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ApiDeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console api: %w", err), instance, metav1.Condition{
			Type:    actions.ApiCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ApiCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Deployment created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func ensureDeployment(tufURL string) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		container := corev1ac.Container().
			WithName(actions.ApiDeploymentName).
			WithImage(images.Registry.Get(images.ConsoleApi)).
			WithArgs("--tuf-repo-url=" + tufURL).
			WithPorts(corev1ac.ContainerPort().
				WithName(actions.ApiPortName).
				WithContainerPort(actions.ApiPort).
				WithProtocol(core.ProtocolTCP)).
			WithLivenessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(actions.ApiPort))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(actions.ApiPort))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath(constants.HealthzPath).
					WithPort(intstr.FromInt32(actions.ApiPort))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		initContainer := corev1ac.Container().
			WithName("wait-for-tuf").
			WithImage(images.Registry.Get(images.TrillianNetcat)).
			WithSecurityContext(traits.SSAContainerSecurityContext()).
			WithEnv(corev1ac.EnvVar().WithName("TUF_URL").WithValue(tufURL)).
			WithCommand("sh", "-c").
			WithArgs(waitForTUFCommand)

		dp.WithSpec(appsv1ac.DeploymentSpec().
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(dp.Labels).
				WithSpec(corev1ac.PodSpec().
					WithContainers(container).
					WithInitContainers(initContainer))))

		return nil
	}
}

func ensureTLS(tlsConfig rhtasv1.TLS) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if err := traits.TLS(tlsConfig, actions.ApiDeploymentName)(podSpec); err != nil {
			return err
		}

		container := kubernetes.FindContainerByName(podSpec, actions.ApiDeploymentName)

		container.WithArgs(
			"--tls-cert="+tls.TLSCertPath,
			"--tls-key="+tls.TLSKeyPath,
		)

		container.ReadinessProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)
		container.LivenessProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)
		container.StartupProbe.HTTPGet.WithScheme(core.URISchemeHTTPS)

		return nil
	}
}
