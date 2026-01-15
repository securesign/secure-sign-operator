package monitor

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/state"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	cutils "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/tls"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	tlsensure "github.com/securesign/operator/internal/utils/tls/ensure"
)

const (
	storageVolumeName = "monitor-storage"
	tufRepoVolumeName = "tuf-repository"
	mountPath         = "/data"
	ctlogLogPrefix    = "trusted-artifact-signer"
)

func NewStatefulSetAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &statefulSetAction{}
}

type statefulSetAction struct {
	action.BaseAction
}

func (i statefulSetAction) Name() string {
	return "statefulset"
}

func (i statefulSetAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	return enabled(instance) && instance.Spec.Monitoring.Enabled && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statefulSetAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	var protocol string
	if tls.UseTlsClient(instance) {
		protocol = "https"
	} else {
		protocol = "http"
	}

	tufServerHost := i.resolveTufUrl(instance)
	ctlogServerHost := fmt.Sprintf("%s://%s.%s.svc", protocol, actions.ComponentName, instance.Namespace)

	labels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.MonitorStatefulSetName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureMonitorStatefulSet(instance, actions.RBACName, labels, ctlogServerHost, tufServerHost),
		i.ensureInitContainer(ctlogServerHost, tufServerHost),
		ensure.ControllerReference[*v1.StatefulSet](instance, i.Client),
		ensure.Labels[*v1.StatefulSet](slices.Collect(maps.Keys(labels)), labels),
		ensure.Optional(
			utils.TlsEnabled(instance),
			i.ensureTLS(instance.Status.TLS, actions.MonitorStatefulSetName),
		),
		func(object *v1.StatefulSet) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
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
		_ = i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}

func (i statefulSetAction) ensureTLS(tlsConfig rhtasv1alpha1.TLS, name string) func(sts *v1.StatefulSet) error {
	return func(sts *v1.StatefulSet) error {
		if err := tlsensure.TLS(tlsConfig, name)(&sts.Spec.Template); err != nil {
			return err
		}
		return nil
	}
}

func (i statefulSetAction) resolveTufUrl(instance *rhtasv1alpha1.CTlog) string {
	if instance.Spec.Monitoring.Tuf.Address != "" {
		url := instance.Spec.Monitoring.Tuf.Address
		if instance.Spec.Monitoring.Tuf.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Monitoring.Tuf.Port)
		}
		return url
	}
	return fmt.Sprintf("http://tuf.%s.svc", instance.Namespace)
}

func (i statefulSetAction) ensureMonitorStatefulSet(instance *rhtasv1alpha1.CTlog, sa string, labels map[string]string, ctlogServerHost string, tufServerHost string) func(*v1.StatefulSet) error {
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
		container.Image = images.Registry.Get(images.CTLogMonitor)

		interval := instance.Spec.Monitoring.TLog.Interval.Duration
		container.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(
				`/ctlog_monitor --file=%s/checkpoint_log.txt --once=false --interval=%s --url=%s/%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
				mountPath, interval.String(), ctlogServerHost, ctlogLogPrefix, tufServerHost, mountPath),
		}

		container.Ports = []core.ContainerPort{
			{
				ContainerPort: actions.MonitorMetricsPort,
				Name:          actions.MonitorMetricsPortName,
				Protocol:      core.ProtocolTCP,
			},
		}

		container.Env = []core.EnvVar{
			{
				Name:  "HOME",
				Value: mountPath,
			},
		}

		if utils.TlsEnabled(instance) {
			container.Env = append(container.Env, core.EnvVar{
				Name:  "SSL_CERT_DIR",
				Value: constants.SecretMountPath,
			})
		}

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
			fmt.Sprintf(`
                echo "Waiting for ctlog-server...";
                until curl -sSf -k %s > /dev/null 2>&1; do
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
