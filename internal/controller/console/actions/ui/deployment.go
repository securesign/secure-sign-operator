package ui

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
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const waitForAPICommand = `curl -fkL --no-progress-meter --output /dev/null --retry 50 --retry-delay 5 --retry-all-errors "${CONSOLE_API_URL}/healthz" && echo 'Successfully connected to Console API'`

func NewDeployAction() action.Action[*rhtasv1.Console] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "ui deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	rekorURL, err := serviceresolver.ResolveExternalServiceUrl(ctx, i.Client, instance.Spec.UI.Rekor, instance.Namespace, &rhtasv1.Rekor{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving Rekor URL: %w", err), instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for Rekor service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.UIDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)),
		ensureDeployment(rekorURL),
		traits.PodRequirements(instance.Spec.UI.PodRequirements, actions.UIDeploymentName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{actions.UIDeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), actions.UIDeploymentName),
			traits.TrustedCA(instance.GetTrustedCA(), actions.UIDeploymentName),
			traits.SecurityContexts(actions.UIDeploymentName),
			traits.ServiceAccount(actions.RBACUIName),
			traits.Optional(
				instance.Status.Api.TLS.CertRef != nil,
				ensureTLS(instance.Namespace),
			),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build console UI deployment: %w", err), instance, metav1.Condition{
			Type: actions.UICondition, Status: metav1.ConditionFalse,
			Reason: state.Failure.String(), Message: err.Error(),
		})
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.UIDeploymentName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console UI: %w", err), instance, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Deployment created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func ensureDeployment(rekorURL string) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		apiURL := fmt.Sprintf("http://%s:%d", actions.ApiDeploymentName, actions.ApiPort)

		container := corev1ac.Container().
			WithName(actions.UIDeploymentName).
			WithImage(images.Registry.Get(images.ConsoleUI)).
			WithEnv(
				corev1ac.EnvVar().WithName("CONSOLE_API_URL").WithValue(apiURL),
				corev1ac.EnvVar().WithName("NEXT_PUBLIC_REKOR_DEFAULT_DOMAIN").WithValue(rekorURL),
			).
			WithPorts(corev1ac.ContainerPort().
				WithName(actions.UIPortName).
				WithContainerPort(actions.UIPort).
				WithProtocol(core.ProtocolTCP)).
			WithLivenessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/").
					WithPort(intstr.FromString(actions.UIPortName))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithReadinessProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/").
					WithPort(intstr.FromString(actions.UIPortName))).
				WithInitialDelaySeconds(0).
				WithPeriodSeconds(10).
				WithTimeoutSeconds(1).
				WithFailureThreshold(3)).
			WithStartupProbe(corev1ac.Probe().
				WithHTTPGet(corev1ac.HTTPGetAction().
					WithPath("/").
					WithPort(intstr.FromString(actions.UIPortName))).
				WithPeriodSeconds(5).
				WithTimeoutSeconds(5).
				WithFailureThreshold(12))

		initContainer := corev1ac.Container().
			WithName("wait-for-api").
			WithImage(images.Registry.Get(images.ConsoleUI)).
			WithSecurityContext(traits.SSAContainerSecurityContext()).
			WithEnv(corev1ac.EnvVar().WithName("CONSOLE_API_URL").WithValue(apiURL)).
			WithCommand("sh", "-c").
			WithArgs(waitForAPICommand)

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

func ensureTLS(namespace string) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		apiURL := fmt.Sprintf("https://%s.%s.svc:%d", actions.ApiDeploymentName, namespace, actions.ApiPort)
		for _, containers := range [][]corev1ac.ContainerApplyConfiguration{podSpec.Containers, podSpec.InitContainers} {
			for i := range containers {
				for j := range containers[i].Env {
					env := &containers[i].Env[j]
					if env.Name != nil && *env.Name == "CONSOLE_API_URL" {
						env.WithValue(apiURL)
					}
				}
			}
		}
		return nil
	}
}
