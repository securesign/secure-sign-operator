package otelcollector

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/images"

	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const storageVolumeName = "otel-config"

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
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.OtelCollectorComponentName, actions.OtelCollectorDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.OtelCollectorDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureCollectorDeployment(instance, actions.RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s deployment: %w", actions.OtelCollectorDeploymentName, err), instance,
			metav1.Condition{
				Type:    actions.OtelCollectorCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.OtelCollectorCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Otel Collector created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}

func (i deployAction) ensureCollectorDeployment(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.OtelCollectorDeploymentName)
		container.Image = images.Registry.Get(images.OtelCollector)

		container.Args = []string{
			"--config=/etc/otel/otel-collector-config.yaml",
		}

		container.Ports = []core.ContainerPort{
			{
				ContainerPort: 4317,
				Name:          "grpc",
				Protocol:      core.ProtocolTCP,
			},
			{
				ContainerPort: 4318,
				Name:          "http",
				Protocol:      core.ProtocolTCP,
			},
			{
				ContainerPort: 9464,
				Name:          "prometheus",
				Protocol:      core.ProtocolTCP,
			},
		}

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
		volumeMount.MountPath = "/etc/otel"

		collectorConfigVolume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, storageVolumeName)
		if collectorConfigVolume.Projected == nil {
			collectorConfigVolume.Projected = &core.ProjectedVolumeSource{}
		}
		collectorConfigVolume.Projected.Sources = []core.VolumeProjection{
			{
				ConfigMap: &core.ConfigMapProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.OtelCollectorConfigRef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  "otel-collector-config.yaml",
							Path: "otel-collector-config.yaml",
							Mode: ptr.To(int32(0666)),
						},
					},
				},
			},
		}
		return nil
	}
}
