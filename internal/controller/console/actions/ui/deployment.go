package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Console] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Console) bool {
	return instance.Spec.Enabled && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {
	labels := labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)
	var (
		result controllerutil.OperationResult
		err    error
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.UIDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureUIDeployment(instance, actions.RBACUIName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.PodRequirements(instance.Spec.UI.PodRequirements, actions.UIDeploymentName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console UI: %w", err), instance, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	// if result != controllerutil.OperationResultNone {
	// 	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: state.Ready.String(),
	// 		Status: metav1.ConditionFalse, Reason: state.Creating.String(), Message: "Deployment created"})
	// 	return i.StatusUpdate(ctx, instance)
	// } else {
	// 	return i.Continue()
	// }

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureUIDeployment(instance *rhtasv1alpha1.Console, sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		replicas := int32(1)
		spec.Replicas = &replicas

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		// --- Init Container: wait-for-backend ---
		initContainer := kubernetes.FindInitContainerByNameOrCreate(
			&template.Spec,
			"wait-for-backend",
		)
		initContainer.Image = images.Registry.Get(images.ConsoleUi)
		initContainer.Command = []string{
			"/bin/sh",
			"-c",
			`max_retries=24
count=0
until curl -sf http://console-api:8080/healthz; do
  if [ $count -ge $max_retries ]; then
    echo "console-api not ready after $((max_retries * 5)) seconds, failing initContainer."
    exit 1
  fi
  echo "Waiting for console-api... ($count/$max_retries)"
  count=$((count + 1))
  sleep 5
done`,
		}

		// --- Main UI Container ---
		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.UIDeploymentName)
		container.Image = images.Registry.Get(images.ConsoleUi)

		apiUrlEnv := kubernetes.FindEnvByNameOrCreate(container, "CONSOLE_API_URL")
		apiUrlEnv.Value = "http://console-api:8080" // TODO

		versionEnv := kubernetes.FindEnvByNameOrCreate(container, "VERSION")
		versionEnv.Value = "v0.1.0" // TODO: from spec

		port := kubernetes.FindPortByNameOrCreate(container, "http")
		port.ContainerPort = 8080
		port.Protocol = core.ProtocolTCP

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(actions.UiServerPort)

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}

		container.ReadinessProbe.HTTPGet.Path = "/"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(actions.UiServerPort)

		return nil
	}
}
