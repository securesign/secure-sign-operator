package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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
	var (
		err    error
		result controllerutil.OperationResult
	)

	l := labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.UIDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		ensure.ControllerReference[*apps.Deployment](instance, i.Client),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(l)), l),
		ensureUIDeployment(instance, l),
		ensureUIInitContainer(instance),
		ensureUIProbes(),
		deployment.PodRequirements(instance.Spec.UI.PodRequirements, actions.UIDeploymentName),
		deployment.Proxy(),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.UIDeploymentName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console UI: %w", err), instance, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
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

func apiTLSEnabled(instance *rhtasv1.Console) bool {
	return instance.Status.Api.TLS.CertRef != nil
}

func ensureUIDeployment(instance *rhtasv1.Console, labels map[string]string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		replicas := int32(1)
		spec.Replicas = &replicas

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = actions.RBACUIName

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.UIDeploymentName)
		container.Image = images.Registry.Get(images.ConsoleUI)

		apiHost := actions.ApiDeploymentName
		scheme := "http"
		if apiTLSEnabled(instance) {
			scheme = "https"
			apiHost = fmt.Sprintf("%s.%s.svc", actions.ApiDeploymentName, instance.Namespace)
		}

		apiURL := kubernetes.FindEnvByNameOrCreate(container, "CONSOLE_API_URL")
		apiURL.Value = fmt.Sprintf("%s://%s:%d", scheme, apiHost, actions.ApiPort)

		port := kubernetes.FindPortByNameOrCreate(container, actions.UIPortName)
		port.ContainerPort = actions.UIPort
		port.Protocol = core.ProtocolTCP

		return nil
	}
}

func ensureUIInitContainer(instance *rhtasv1.Console) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		apiHost := actions.ApiDeploymentName
		scheme := "http"
		curlFlags := "-sf"
		if apiTLSEnabled(instance) {
			scheme = "https"
			curlFlags = "-sfk"
			apiHost = fmt.Sprintf("%s.%s.svc", actions.ApiDeploymentName, instance.Namespace)
		}

		initContainer := kubernetes.FindInitContainerByNameOrCreate(&dp.Spec.Template.Spec, "wait-for-api")
		initContainer.Image = images.Registry.Get(images.ConsoleUI)
		initContainer.Command = []string{"/bin/sh", "-c",
			fmt.Sprintf(`max_retries=30
count=0
until curl %s %s://%s:%d/healthz; do
  if [ $count -ge $max_retries ]; then
    echo "console-api not ready after $((max_retries * 5)) seconds, failing initContainer."
    exit 1
  fi
  echo "Waiting for console-api... ($count/$max_retries)"
  count=$((count + 1))
  sleep 5
done`, curlFlags, scheme, apiHost, actions.ApiPort)}

		return nil
	}
}

func ensureUIProbes() func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.UIDeploymentName)

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/"
		container.LivenessProbe.HTTPGet.Port = intstr.FromString(actions.UIPortName)
		container.LivenessProbe.InitialDelaySeconds = 15
		container.LivenessProbe.PeriodSeconds = 10

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromString(actions.UIPortName)
		container.ReadinessProbe.InitialDelaySeconds = 5
		container.ReadinessProbe.PeriodSeconds = 10

		return nil
	}
}
