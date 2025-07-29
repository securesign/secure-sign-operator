package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/images"

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const storageVolumeName = "monitor-storage"

func NewStatefulSetAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &statefulSetAction{}
}

type statefulSetAction struct {
	action.BaseAction
}

func (i statefulSetAction) Name() string {
	return "statefulset"
}

func (i statefulSetAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
}

func (i statefulSetAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	rekorServerHost := fmt.Sprintf("http://%s.%s.svc", actions.ServerComponentName, instance.Namespace)

	labels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorStatefulSetName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureMonitorStatefulSet(instance, actions.RBACName, labels, rekorServerHost),
		i.ensureInitContainer(rekorServerHost),
		ensure.ControllerReference[*v1.StatefulSet](instance, i.Client),
		ensure.Labels[*v1.StatefulSet](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s statefulset: %w", actions.MonitorStatefulSetName, err), instance,
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
		_ = i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}

func (i statefulSetAction) ensureMonitorStatefulSet(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string, rekorServerHost string) func(*v1.StatefulSet) error {
	return func(ss *v1.StatefulSet) error {

		spec := &ss.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.MonitorStatefulSetName)
		container.Image = images.Registry.Get(images.RekorMonitor)

		interval := instance.Spec.Monitoring.TLog.Interval.Duration
		container.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`/rekor_monitor --file=/data/checkpoint_log.txt --once=false --interval=%s --url=%s`, interval.String(), rekorServerHost),
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

		spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: storageVolumeName,
				},
				Spec: core.PersistentVolumeClaimSpec{
					AccessModes: []core.PersistentVolumeAccessMode{
						core.ReadWriteOnce,
					},
					Resources: core.VolumeResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceStorage: resource.MustParse("5Mi"),
						},
					},
				},
			},
		}
		return nil
	}
}

func (i statefulSetAction) ensureInitContainer(rekorServerHost string) func(*v1.StatefulSet) error {
	return func(ss *v1.StatefulSet) error {
		initContainer := kubernetes.FindInitContainerByNameOrCreate(&ss.Spec.Template.Spec, "wait-for-rekor-server")
		initContainer.Image = images.Registry.Get(images.RekorMonitor)

		initContainer.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`until curl -sf %s > /dev/null 2>&1; do echo 'Waiting for rekor-server to be ready...'; sleep 5; done`, rekorServerHost),
		}

		return nil
	}
}
