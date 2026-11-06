package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	storageVolumeName = "monitor-storage"
	tufRepoVolumeName = "tuf-repository"
	mountPath         = "/data"
	initContainerName = "tuf-init"
)

func NewStatefulSetAction() action.Action[*rhtasv1.Rekor] {
	return &statefulSetAction{}
}

type statefulSetAction struct {
	action.BaseAction
}

func (i statefulSetAction) Name() string {
	return "statefulset"
}

func (i statefulSetAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statefulSetAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	tufServerHost, err := serviceresolver.ResolveInternalServiceUrl(ctx, i.Client, instance.Spec.Monitoring.Tuf, instance.Namespace, &rhtasv1.Tuf{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve TUF url: %w", err), instance, metav1.Condition{
			Type:               actions.MonitorCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for TUF service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}
	rekorServerHost, err := serviceresolver.Resolve(instance)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	labels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	ss, err := traits.Apply(
		appsv1ac.StatefulSet(actions.MonitorStatefulSetName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.StatefulSetApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.StatefulSetApplyConfiguration](labels),
		i.createMonitorStatefulSet(instance, rekorServerHost, tufServerHost),
		traits.WithStatefulSetPodSpecTraits(
			traits.ServiceAccount(actions.RBACMonitorName),
			traits.SecurityContexts(actions.MonitorStatefulSetName, initContainerName),
			traits.GODEBUG(instance.GetAnnotations(), actions.MonitorStatefulSetName),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build %s statefulset: %w", actions.MonitorStatefulSetName, err), instance,
			metav1.Condition{
				Type:    actions.MonitorCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, ss,
		&apps.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorStatefulSetName,
				Namespace: instance.Namespace,
			},
		},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s statefulset: %w", actions.MonitorStatefulSetName, err), instance,
			metav1.Condition{
				Type:    actions.MonitorCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.MonitorCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Monitor created",
		})
		if _, err := i.PersistStatus(ctx, instance); err != nil {
			return i.Error(ctx, err, instance)
		}
	}
	return i.Continue()
}

// createMonitorStatefulSet builds the complete desired monitor pod template from
// scratch so that fields the operator stops declaring are pruned by SSA.
func (i statefulSetAction) createMonitorStatefulSet(instance *rhtasv1.Rekor, rekorServerHost string, tufServerHost string) traits.Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		interval := 10 * time.Minute
		if instance.Spec.Monitoring.TLog.Interval != nil && instance.Spec.Monitoring.TLog.Interval.Duration > 0 {
			interval = instance.Spec.Monitoring.TLog.Interval.Duration
		}

		container := corev1ac.Container().
			WithName(actions.MonitorStatefulSetName).
			WithImage(images.Registry.Get(images.RekorMonitor)).
			WithCommand(
				"/bin/sh",
				"-c",
				fmt.Sprintf(
					`/rekor_monitor --file=%s/checkpoint_log.txt --once=false --interval=%s --url=%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
					mountPath, interval.String(), rekorServerHost, tufServerHost, mountPath),
			).
			WithPorts(corev1ac.ContainerPort().
				WithContainerPort(actions.MonitorMetricsPort).
				WithName(actions.MetricsPortName).
				WithProtocol(core.ProtocolTCP)).
			WithEnv(corev1ac.EnvVar().
				WithName("HOME").
				WithValue(mountPath)).
			WithVolumeMounts(corev1ac.VolumeMount().
				WithName(storageVolumeName).
				WithMountPath(mountPath))

		ss.WithSpec(appsv1ac.StatefulSetSpec().
			WithReplicas(1).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(ss.Labels)).
			// Declared at the API server defaults the StatefulSet was created with.
			// The partition must be explicit: it is only defaulted when the type is
			// unset, so declaring the type alone would prune it after migration.
			WithUpdateStrategy(appsv1ac.StatefulSetUpdateStrategy().
				WithType(apps.RollingUpdateStatefulSetStrategyType).
				WithRollingUpdate(appsv1ac.RollingUpdateStatefulSetStrategy().WithPartition(0))).
			// podManagementPolicy is immutable and must not change.
			WithPodManagementPolicy(apps.OrderedReadyPodManagement).
			WithVolumeClaimTemplates(monitorStorageClaimTemplate()).
			WithTemplate(corev1ac.PodTemplateSpec().
				WithLabels(ss.Labels).
				WithSpec(corev1ac.PodSpec().
					WithInitContainers(initContainer(rekorServerHost, tufServerHost)).
					WithContainers(container))))
		return nil
	}
}

func initContainer(rekorServerHost string, tufHost string) *corev1ac.ContainerApplyConfiguration {
	return corev1ac.Container().
		WithName(initContainerName).
		WithImage(images.Registry.Get(images.RekorMonitor)).
		WithVolumeMounts(corev1ac.VolumeMount().
			WithName(storageVolumeName).
			WithMountPath(mountPath)).
		WithCommand(
			"/bin/sh",
			"-c",
			fmt.Sprintf(`
                echo "Waiting for rekor-server...";
                until curl -sf %s > /dev/null 2>&1; do
                    echo "rekor-server not ready...";
                    sleep 5;
                done;
                echo "Waiting for TUF server...";
                until curl %s > /dev/null 2>&1; do
                    echo "TUF server not ready...";
                    sleep 5;
                done;
                echo "Downloading root.json";
                curl %s/root.json > %s/root.json
                echo "tuf-init completed."
            `, rekorServerHost, tufHost, tufHost, mountPath),
		)
}

// monitorStorageClaimTemplate must stay identical to the template existing
// StatefulSets were created with: volumeClaimTemplates is immutable. It is built
// without corev1ac.PersistentVolumeClaim so no kind/apiVersion/namespace is sent.
func monitorStorageClaimTemplate() *corev1ac.PersistentVolumeClaimApplyConfiguration {
	return (&corev1ac.PersistentVolumeClaimApplyConfiguration{}).
		WithName(storageVolumeName).
		WithSpec(corev1ac.PersistentVolumeClaimSpec().
			WithAccessModes(core.ReadWriteOnce).
			WithResources(corev1ac.VolumeResourceRequirements().
				WithRequests(core.ResourceList{
					core.ResourceStorage: resource.MustParse("5Mi"),
				})))
}
