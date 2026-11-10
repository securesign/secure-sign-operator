package monitor

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testenvhelper "github.com/securesign/operator/internal/testing/envtest"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestMonitorStatefulSetEnvtest runs the action against a real API server, which
// (unlike the fake client) enforces StatefulSet immutability, applies defaults,
// and tracks managed fields exactly. No controller manager runs, so only the
// action under test touches the objects.
func TestMonitorStatefulSetEnvtest(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: testenvhelper.FindBinaryAssetsDir(),
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Errorf("stopping envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rhtasv1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	t.Run("no-op reconcile does not write or reset status", func(t *testing.T) {
		g := NewWithT(t)
		instance := createEnvtestInstance(t, c, "noop")

		first := handleEnvtest(t, c, instance)
		markMonitorReady(t, c, instance)

		second := handleEnvtest(t, c, instance)
		g.Expect(second.ResourceVersion).To(Equal(first.ResourceVersion), "no-op apply must not write")
		g.Expect(monitorReason(t, c, instance)).To(Equal(state.Ready.String()), "no-op apply must not reset status")
	})

	t.Run("upgrade from legacy object keeps spec and does not roll pods", func(t *testing.T) {
		g := NewWithT(t)
		instance := createEnvtestInstance(t, c, "upgrade")
		legacy := createLegacy(t, c, instance)
		g.Expect(managers(legacy)).To(ContainElement(kubernetes.LegacyFieldManager))

		migrated := handleEnvtest(t, c, instance)
		g.Expect(migrated.Generation).To(Equal(legacy.Generation),
			"migration must not change the spec (a template change would roll the monitor pod)")
		g.Expect(migrated.Spec).To(Equal(legacy.Spec))
		g.Expect(managers(migrated)).ToNot(ContainElement(kubernetes.LegacyFieldManager))
		g.Expect(managers(migrated)).To(ContainElement(fieldManager))
	})

	t.Run("upgrade prunes fields left by the legacy manager", func(t *testing.T) {
		g := NewWithT(t)
		instance := createEnvtestInstance(t, c, "prune")
		legacy := createLegacy(t, c, instance, func(ss *apps.StatefulSet) error {
			ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env,
				core.EnvVar{Name: "STALE", Value: "left-by-old-operator"})
			return nil
		})
		g.Expect(findEnv("STALE", legacy.Spec.Template.Spec.Containers[0].Env)).ToNot(BeNil())

		migrated := handleEnvtest(t, c, instance)
		container := monitorContainer(migrated.Spec.Template.Spec.Containers)
		g.Expect(container.Env).To(ConsistOf(core.EnvVar{Name: "HOME", Value: mountPath}), "only the stale entry is removed")
	})

	t.Run("updates keep foreign fields", func(t *testing.T) {
		g := NewWithT(t)
		instance := createEnvtestInstance(t, c, "foreign")
		created := handleEnvtest(t, c, instance)

		// What `kubectl annotate` and `kubectl rollout restart` do.
		patch := client.MergeFrom(created.DeepCopy())
		created.Annotations = map[string]string{"example.com/owner": "other"}
		created.Spec.Template.Annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": "2026-10-26T00:00:00Z"}
		g.Expect(c.Patch(t.Context(), created, patch, client.FieldOwner("kubectl"))).To(Succeed())

		// Change the CR so the next apply actually updates the StatefulSet.
		instance.Spec.Monitoring.TLog.Interval = &metav1.Duration{Duration: 5 * time.Minute}
		updated := handleEnvtest(t, c, instance)

		g.Expect(updated.Annotations).To(HaveKeyWithValue("example.com/owner", "other"))
		g.Expect(updated.Spec.Template.Annotations).To(HaveKeyWithValue("kubectl.kubernetes.io/restartedAt", "2026-10-26T00:00:00Z"))
		g.Expect(managers(updated)).To(ContainElements(fieldManager, "kubectl"))
	})
}

func createEnvtestInstance(t *testing.T, c client.Client, name string) *rhtasv1.Rekor {
	t.Helper()
	ctx := t.Context()
	ns := &core.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitor-" + name}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace: %v", err)
	}

	instance := createMonitorInstance()
	instance.Name = name
	instance.Namespace = ns.Name
	instance.UID = ""
	instance.Status = rhtasv1.RekorStatus{}
	if err := c.Create(ctx, instance); err != nil {
		t.Fatalf("creating Rekor: %v", err)
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	if err := c.Status().Update(ctx, instance); err != nil {
		t.Fatalf("updating Rekor status: %v", err)
	}
	return instance
}

func handleEnvtest(t *testing.T, c client.Client, instance *rhtasv1.Rekor) *apps.StatefulSet {
	t.Helper()
	ss, err := handleStatefulSet(t, c, instance)
	if err != nil {
		t.Fatalf("handling statefulset: %v", err)
	}
	return ss
}

// markMonitorReady simulates a later action marking the monitor ready.
func markMonitorReady(t *testing.T, c client.Client, instance *rhtasv1.Rekor) {
	t.Helper()
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(instance), instance); err != nil {
		t.Fatalf("getting Rekor: %v", err)
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.MonitorCondition,
		Status: metav1.ConditionTrue,
		Reason: state.Ready.String(),
	})
	if err := c.Status().Update(t.Context(), instance); err != nil {
		t.Fatalf("updating Rekor status: %v", err)
	}
}

func monitorReason(t *testing.T, c client.Client, instance *rhtasv1.Rekor) string {
	t.Helper()
	live := &rhtasv1.Rekor{}
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(instance), live); err != nil {
		t.Fatalf("getting Rekor: %v", err)
	}
	condition := meta.FindStatusCondition(live.Status.Conditions, actions.MonitorCondition)
	if condition == nil {
		return ""
	}
	return condition.Reason
}

// createLegacy creates the StatefulSet exactly as the pre-SSA operator did:
// controllerutil.CreateOrUpdate (an Update operation) under the "manager" field
// manager, using a frozen copy of the old ensure functions.
func createLegacy(t *testing.T, c client.Client, instance *rhtasv1.Rekor, extra ...func(*apps.StatefulSet) error) *apps.StatefulSet {
	t.Helper()
	ctx := t.Context()
	legacyClient := client.WithFieldOwner(c, kubernetes.LegacyFieldManager)

	rekorServerHost := fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace)
	l := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	fns := slices.Concat([]func(*apps.StatefulSet) error{
		legacyEnsureMonitorStatefulSet(instance, actions.RBACMonitorName, l, rekorServerHost, tufURL),
		legacyEnsureInitContainer(rekorServerHost, tufURL),
		ensure.ControllerReference[*apps.StatefulSet](instance, c),
		ensure.Labels[*apps.StatefulSet](slices.Collect(maps.Keys(l)), l),
		func(object *apps.StatefulSet) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
		},
		func(object *apps.StatefulSet) error {
			return ensure.GODEBUG(instance.GetAnnotations())(&object.Spec.Template.Spec)
		},
	}, extra)

	ss := &apps.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: actions.MonitorStatefulSetName, Namespace: instance.Namespace}}
	if _, err := kubernetes.CreateOrUpdate(ctx, legacyClient, ss, fns...); err != nil {
		t.Fatalf("creating legacy statefulset: %v", err)
	}
	return getStatefulSet(ctx, t, c, instance)
}

func getStatefulSet(ctx context.Context, t *testing.T, c client.Client, instance *rhtasv1.Rekor) *apps.StatefulSet {
	t.Helper()
	ss := &apps.StatefulSet{}
	if err := c.Get(ctx, client.ObjectKey{Name: actions.MonitorStatefulSetName, Namespace: instance.Namespace}, ss); err != nil {
		t.Fatalf("getting statefulset: %v", err)
	}
	return ss
}

// legacyEnsureMonitorStatefulSet is a frozen copy of the pre-SSA implementation.
func legacyEnsureMonitorStatefulSet(instance *rhtasv1.Rekor, sa string, labels map[string]string, rekorServerHost string, tufServerHost string) func(*apps.StatefulSet) error {
	return func(ss *apps.StatefulSet) error {
		spec := &ss.Spec
		spec.Replicas = ptr.To[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.MonitorStatefulSetName)
		container.Image = images.Registry.Get(images.RekorMonitor)

		interval := 10 * time.Minute
		if instance.Spec.Monitoring.TLog.Interval != nil && instance.Spec.Monitoring.TLog.Interval.Duration > 0 {
			interval = instance.Spec.Monitoring.TLog.Interval.Duration
		}
		container.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(
				`/rekor_monitor --file=%s/checkpoint_log.txt --once=false --interval=%s --url=%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
				mountPath, interval.String(), rekorServerHost, tufServerHost, mountPath),
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

// legacyEnsureInitContainer is a frozen copy of the pre-SSA implementation.
func legacyEnsureInitContainer(rekorServerHost string, tufHost string) func(*apps.StatefulSet) error {
	return func(ss *apps.StatefulSet) error {
		initContainer := kubernetes.FindInitContainerByNameOrCreate(&ss.Spec.Template.Spec, "tuf-init")
		initContainer.Image = images.Registry.Get(images.RekorMonitor)
		volumeMount := kubernetes.FindVolumeMountByNameOrCreate(initContainer, storageVolumeName)
		volumeMount.MountPath = mountPath
		initContainer.Command = []string{
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
		}

		return nil
	}
}
