package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

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
	v2 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const storageVolumeName = "monitor-storage"

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

	// Rekor Ingress
	ok := types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}
	rekorIngress := &v2.Ingress{}
	if err := i.Client.Get(ctx, ok, rekorIngress); err != nil {
		return i.Error(ctx, fmt.Errorf("could not find rekor ingress: %w", err), instance)
	}
	rekorServerHost := "https://" + rekorIngress.Spec.Rules[0].Host

	labels := labels.For(actions.MonitorComponentName, actions.MonitorDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureMonitorDeployment(actions.RBACName, labels, rekorServerHost),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s deployment: %w", actions.MonitorDeploymentName, err), instance,
			metav1.Condition{
				Type:    actions.MonitorCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.MonitorCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Monitor created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}

func (i deployAction) ensureMonitorDeployment(sa string, labels map[string]string, rekorServerHost string) func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.MonitorDeploymentName)
		container.Image = images.Registry.Get(images.RekorMonitor)

		otelCollectorEndpoint := "http://" + actions.OtelCollectorComponentName + ":" + strconv.Itoa(actions.OtelCollectorGrpcPort)
		container.Env = []core.EnvVar{
			{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: otelCollectorEndpoint},
			{Name: "REKOR_SERVER_ENDPOINT", Value: rekorServerHost},
			{Name: "CHECK_INTERVAL_SECONDS", Value: "5"},
		}

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
		volumeMount.MountPath = "/data"

		volume := kubernetes.FindVolumeByNameOrCreate(&template.Spec, storageVolumeName)
		if volume.EmptyDir == nil {
			volume.EmptyDir = &core.EmptyDirVolumeSource{}
		}
		return nil
	}
}
