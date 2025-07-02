package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	cutils "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	rekorServerHost := fmt.Sprintf("http://%s.%s.svc", actions.ServerComponentName, instance.Namespace)

	labels := labels.For(actions.MonitorComponentName, actions.MonitorDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureMonitorDeployment(instance, actions.RBACName, labels, rekorServerHost),
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

func (i deployAction) ensureMonitorDeployment(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string, rekorServerHost string) func(*v1.Deployment) error {
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

		container.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`
				until curl -sf %s > /dev/null 2>&1; do
					echo 'Waiting for rekor-server to be ready...';
					sleep 5;
				done;
				exec /rekor_monitor --file=/data/checkpoint_log.txt --once=false --interval=%s --url=%s
			`, rekorServerHost, instance.Spec.Monitoring.TLog.Interval.Duration.String(), rekorServerHost),
		}

		container.Ports = []core.ContainerPort{
			{
				ContainerPort: actions.MonitorMetricsPort,
				Name:          actions.MonitorMetricsPortName,
				Protocol:      core.ProtocolTCP,
			},
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
