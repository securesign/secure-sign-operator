package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1 "github.com/securesign/operator/api/v1"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
)

const (
	storageVolumeName = "monitor-storage"
	tufRepoVolumeName = "tuf-repository"
	mountPath         = "/data"
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
	var (
		err    error
		result controllerutil.OperationResult
	)

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

	// Deliberately not serviceresolver.Resolve/ResolveExternalServiceUrl like Rekor's monitor:
	// unlike Rekor (which validates a signed checkpoint), this monitor compares the log URL
	// byte-for-byte against the URL stored in TUF metadata, so it must dial the exact external
	// URL once ingress is enabled. ResolveExternalServiceUrl requires CTlog.Ready, but this
	// action runs before Ready (server and monitor share one CR), which would deadlock.
	ctlogServerHost, err := actions.ResolveUrl(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	labels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorStatefulSetName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureMonitorStatefulSet(instance, actions.RBACMonitorName, labels, ctlogServerHost, tufServerHost),
		i.ensureInitContainer(ctlogServerHost, tufServerHost),
		ensure.ControllerReference[*v1.StatefulSet](instance, i.Client),
		ensure.Labels[*v1.StatefulSet](slices.Collect(maps.Keys(labels)), labels),
		func(object *v1.StatefulSet) error {
			return tlsensure.TrustedCA(instance.GetTrustedCA(), actions.MonitorStatefulSetName)(&object.Spec.Template)
		},
		func(object *v1.StatefulSet) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
		},
		func(object *v1.StatefulSet) error {
			return ensure.GODEBUG(instance.GetAnnotations())(&object.Spec.Template.Spec)
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s statefulset: %w", actions.MonitorStatefulSetName, err), instance,
			metav1.Condition{
				Type:    actions.MonitorCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
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

func (i statefulSetAction) ensureMonitorStatefulSet(instance *rhtasv1.CTlog, sa string, labels map[string]string, ctlogServerHost string, tufServerHost string) func(*v1.StatefulSet) error {
	return func(ss *v1.StatefulSet) error {

		spec := &ss.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.MonitorStatefulSetName)
		container.Image = images.Registry.Get(images.CTLogMonitor)

		interval := 10 * time.Minute
		if instance.Spec.Monitoring.TLog.Interval != nil && instance.Spec.Monitoring.TLog.Interval.Duration > 0 {
			interval = instance.Spec.Monitoring.TLog.Interval.Duration
		}
		container.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(
				`/ctlog_monitor --file=%s/checkpoint_log.txt --once=false --interval=%s --url=%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
				mountPath, interval.String(), ctlogServerHost, tufServerHost, mountPath),
		}

		container.Ports = []core.ContainerPort{
			{
				ContainerPort: actions.MonitorMetricsPort,
				Name:          actions.MetricsPortName,
				Protocol:      core.ProtocolTCP,
			},
		}

		homeEnv := kubernetes.FindEnvByNameOrCreate(container, "HOME")
		homeEnv.Value = mountPath

		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(container, storageVolumeName)
		volumeMount.MountPath = mountPath

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

func (i statefulSetAction) ensureInitContainer(ctlogServerHost string, tufHost string) func(*v1.StatefulSet) error {
	return func(ss *v1.StatefulSet) error {
		initContainer := kubernetes.FindInitContainerByNameOrCreate(&ss.Spec.Template.Spec, "tuf-init")

		initContainer.Image = images.Registry.Get(images.CTLogMonitor)
		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(initContainer, storageVolumeName)
		volumeMount.MountPath = mountPath

		initContainer.Command = []string{
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
		}
		return nil
	}
}
