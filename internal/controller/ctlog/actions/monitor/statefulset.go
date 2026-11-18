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
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
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

func NewStatefulSetAction() action.Action[*rhtasv1.CTlog] {
	return &statefulSetAction{}
}

type statefulSetAction struct {
	action.BaseAction
}

func (i statefulSetAction) Name() string {
	return "statefulset"
}

func (i statefulSetAction) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return enabled(instance) && utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statefulSetAction) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
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

	// The preceding status action resolves the full URL, including the ingress
	// host and active log path. The monitor must use the same URL as TUF metadata.
	ctlogServerHost := instance.Status.URL

	labels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	ss, err := traits.Apply(
		appsv1ac.StatefulSet(actions.MonitorStatefulSetName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.StatefulSetApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.StatefulSetApplyConfiguration](labels),
		i.createMonitorStatefulSet(instance, ctlogServerHost, tufServerHost),
		traits.WithStatefulSetPodSpecTraits(
			traits.ServiceAccount(actions.RBACMonitorName),
			// The init container dials the CTlog URL, which is the external ingress
			// URL when ingress is enabled, so it needs the proxy settings too.
			traits.Proxy([]string{actions.MonitorStatefulSetName, initContainerName}),
			traits.TrustedCA(instance.GetTrustedCA(), actions.MonitorStatefulSetName),
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
func (i statefulSetAction) createMonitorStatefulSet(instance *rhtasv1.CTlog, ctlogServerHost string, tufServerHost string) traits.Trait[*appsv1ac.StatefulSetApplyConfiguration] {
	return func(ss *appsv1ac.StatefulSetApplyConfiguration) error {
		interval := 10 * time.Minute
		if instance.Spec.Monitoring.TLog.Interval != nil && instance.Spec.Monitoring.TLog.Interval.Duration > 0 {
			interval = instance.Spec.Monitoring.TLog.Interval.Duration
		}

		container := corev1ac.Container().
			WithName(actions.MonitorStatefulSetName).
			WithImage(images.Registry.Get(images.CTLogMonitor)).
			WithCommand(
				"/bin/sh",
				"-c",
				fmt.Sprintf(
					`/ctlog_monitor --file=%s/checkpoint_log.txt --once=false --interval=%s --url=%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
					mountPath, interval.String(), ctlogServerHost, tufServerHost, mountPath),
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
					WithInitContainers(initContainer(ctlogServerHost, tufServerHost)).
					WithContainers(container))))
		return nil
	}
}

func initContainer(ctlogServerHost string, tufHost string) *corev1ac.ContainerApplyConfiguration {
	return corev1ac.Container().
		WithName(initContainerName).
		WithImage(images.Registry.Get(images.CTLogMonitor)).
		WithVolumeMounts(corev1ac.VolumeMount().
			WithName(storageVolumeName).
			WithMountPath(mountPath)).
		WithCommand(
			"/bin/sh",
			"-c",
			// use common endpoint to check prefix availability (see https://datatracker.ietf.org/doc/html/rfc6962)
			fmt.Sprintf(`
                echo "Waiting for ctlog-server...";
                until curl -sSf -k %s/ct/v1/get-sth > /dev/null 2>&1; do
                    echo "ctlog-server not ready...";
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
            `, ctlogServerHost, tufHost, tufHost, mountPath),
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
