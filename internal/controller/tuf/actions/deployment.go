package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
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
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	return state.FromInstance(tuf, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	labels := labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)

	var (
		result controllerutil.OperationResult
		err    error
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      tufConstants.DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.createTufDeployment(instance, tufConstants.RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.PodRequirements(instance.Spec.PodRequirements, tufConstants.ContainerName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.ReadyCondition,
			Status: metav1.ConditionFalse, Reason: state.Creating.String(), Message: "Deployment created",
			ObservedGeneration: instance.Generation})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) createTufDeployment(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}
		spec.Strategy = v1.DeploymentStrategy{
			Type: "Recreate",
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, tufConstants.VolumeName)
		if volume.PersistentVolumeClaim == nil {
			volume.PersistentVolumeClaim = &core.PersistentVolumeClaimVolumeSource{}
		}
		volume.PersistentVolumeClaim.ClaimName = instance.Status.PvcName

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, tufConstants.ContainerName)
		container.Image = images.Registry.Get(images.HttpServer)

		port := kubernetes.FindPortByNameOrCreate(container, tufConstants.PortName)
		port.ContainerPort = 8080
		port.Protocol = core.ProtocolTCP

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, tufConstants.VolumeName)
		volumeMount.MountPath = "/var/www/html"
		// let user upload manual update using `oc rsync` command
		volumeMount.ReadOnly = false

		// Liveness probe - verifies HTTP server process is alive
		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.TCPSocket == nil {
			container.LivenessProbe.TCPSocket = &core.TCPSocketAction{}
		}
		container.LivenessProbe.Exec = nil // Clear any existing exec probe
		container.LivenessProbe.TCPSocket.Port = intstr.FromInt32(8080)
		container.LivenessProbe.InitialDelaySeconds = 15
		container.LivenessProbe.PeriodSeconds = 10
		container.LivenessProbe.TimeoutSeconds = 1
		container.LivenessProbe.FailureThreshold = 3

		// Readiness probe - verifies TUF repository is serving content
		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/root.json"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(8080)
		container.ReadinessProbe.HTTPGet.Scheme = "HTTP"
		container.ReadinessProbe.InitialDelaySeconds = 5
		container.ReadinessProbe.PeriodSeconds = 10
		container.ReadinessProbe.TimeoutSeconds = 1
		container.ReadinessProbe.FailureThreshold = 3

		return nil
	}
}
