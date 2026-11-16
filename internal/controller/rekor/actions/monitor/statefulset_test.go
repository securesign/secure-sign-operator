package monitor

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	_ "github.com/securesign/operator/internal/controller/rekor/serviceresolver"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tufURL       = "http://tuf.default.svc"
	fieldManager = kubernetes.FieldManager + "/rekor-controller"
)

func createMonitorInstance() *rhtasv1.Rekor {
	return &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rekor",
			Namespace: "default",
			UID:       "rekor-uid",
		},
		Spec: rhtasv1.RekorSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				TLog: rhtasv1.TlogMonitoring{
					Enabled:  ptr.To(true),
					Interval: &metav1.Duration{Duration: 10 * time.Minute},
				},
				Tuf: rhtasv1.ServiceReference{URL: tufURL},
			},
		},
		Status: rhtasv1.RekorStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Status: metav1.ConditionFalse,
					Reason: state.Creating.String(),
				},
			},
		},
	}
}

func createMonitorClient(objects ...client.Object) client.Client {
	return testAction.FakeClientBuilder().
		WithReturnManagedFields().
		WithObjects(objects...).
		WithStatusSubresource(&rhtasv1.Rekor{}).
		Build()
}

func handleStatefulSet(t *testing.T, c client.Client, instance *rhtasv1.Rekor) (*apps.StatefulSet, error) {
	t.Helper()
	ctx := t.Context()

	result := testAction.PrepareAction(kubernetes.WithFieldManager(c, fieldManager), NewStatefulSetAction()).Handle(ctx, instance)
	if result != nil && result.Err != nil {
		return nil, result.Err
	}

	ss := &apps.StatefulSet{}
	if err := c.Get(ctx, client.ObjectKey{Name: actions.MonitorStatefulSetName, Namespace: instance.Namespace}, ss); err != nil {
		return nil, err
	}
	return ss, nil
}

func monitorContainer(containers []core.Container) *core.Container {
	for i := range containers {
		if containers[i].Name == actions.MonitorStatefulSetName {
			return &containers[i]
		}
	}
	return nil
}

func findEnv(name string, envs []core.EnvVar) *core.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func managers(ss *apps.StatefulSet) []string {
	result := make([]string, 0, len(ss.ManagedFields))
	for _, mf := range ss.ManagedFields {
		result = append(result, mf.Manager)
	}
	return result
}

func TestMonitorStatefulSetCreate(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)

	ss, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	l := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	g.Expect(ss.Labels).To(Equal(l))
	g.Expect(ss.OwnerReferences).To(HaveLen(1))
	g.Expect(ss.OwnerReferences[0].Name).To(Equal(instance.Name))
	g.Expect(ss.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))

	g.Expect(ss.Spec.Replicas).To(HaveValue(Equal(int32(1))))
	g.Expect(ss.Spec.Selector.MatchLabels).To(Equal(l))
	g.Expect(ss.Spec.Template.Labels).To(Equal(l))
	g.Expect(ss.Spec.ServiceName).To(BeEmpty(), "serviceName is immutable and was never set")
	g.Expect(ss.Spec.UpdateStrategy.Type).To(Equal(apps.RollingUpdateStatefulSetStrategyType))
	g.Expect(ss.Spec.UpdateStrategy.RollingUpdate.Partition).To(HaveValue(Equal(int32(0))))
	g.Expect(ss.Spec.PodManagementPolicy).To(Equal(apps.OrderedReadyPodManagement))

	g.Expect(ss.Spec.VolumeClaimTemplates).To(HaveLen(1))
	pvc := ss.Spec.VolumeClaimTemplates[0]
	g.Expect(pvc.Name).To(Equal(storageVolumeName))
	g.Expect(pvc.Spec.AccessModes).To(ConsistOf(core.ReadWriteOnce))
	g.Expect(pvc.Spec.Resources.Requests.Storage().String()).To(Equal("5Mi"))

	podSpec := ss.Spec.Template.Spec
	g.Expect(podSpec.ServiceAccountName).To(Equal(actions.RBACMonitorName))

	container := monitorContainer(podSpec.Containers)
	g.Expect(container).ToNot(BeNil())
	g.Expect(container.Image).To(Equal(images.Registry.Get(images.RekorMonitor)))
	g.Expect(container.Command).To(HaveLen(3))
	g.Expect(container.Command[2]).To(ContainSubstring("--interval=10m0s"))
	g.Expect(container.Command[2]).To(ContainSubstring("--url=http://rekor-server.default.svc"))
	g.Expect(container.Command[2]).To(ContainSubstring("--tuf-repository=" + tufURL))
	g.Expect(container.Ports).To(ConsistOf(core.ContainerPort{
		Name:          actions.MetricsPortName,
		ContainerPort: actions.MonitorMetricsPort,
		Protocol:      core.ProtocolTCP,
	}))
	g.Expect(container.Env).To(ConsistOf(core.EnvVar{Name: "HOME", Value: mountPath}))
	g.Expect(container.VolumeMounts).To(ConsistOf(core.VolumeMount{Name: storageVolumeName, MountPath: mountPath}))

	g.Expect(podSpec.InitContainers).To(HaveLen(1))
	initC := podSpec.InitContainers[0]
	g.Expect(initC.Name).To(Equal(initContainerName))
	g.Expect(initC.Image).To(Equal(images.Registry.Get(images.RekorMonitor)))
	g.Expect(initC.Command[2]).To(ContainSubstring("curl " + tufURL + "/root.json > /data/root.json"))
	g.Expect(initC.VolumeMounts).To(ConsistOf(core.VolumeMount{Name: storageVolumeName, MountPath: mountPath}))

	g.Expect(managers(ss)).To(ConsistOf(fieldManager))

	condition := meta.FindStatusCondition(instance.Status.Conditions, actions.MonitorCondition)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Reason).To(Equal(state.Creating.String()))
	g.Expect(condition.Message).To(Equal("Monitor created"))
}

func TestMonitorStatefulSetUpdate(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)

	_, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.MonitorCondition,
		Status: metav1.ConditionTrue,
		Reason: state.Ready.String(),
	})

	instance.Spec.Monitoring.TLog.Interval = &metav1.Duration{Duration: 5 * time.Minute}
	updated, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	container := monitorContainer(updated.Spec.Template.Spec.Containers)
	g.Expect(container.Command[2]).To(ContainSubstring("--interval=5m0s"))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, actions.MonitorCondition).Reason).
		To(Equal(state.Creating.String()))
}

func TestMonitorStatefulSetGODEBUGPruning(t *testing.T) {
	t.Setenv("GODEBUG", "")
	g := NewWithT(t)
	instance := createMonitorInstance()
	instance.Annotations = map[string]string{annotations.Godebug: "fips140=only"}
	c := createMonitorClient(instance)

	ss, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	container := monitorContainer(ss.Spec.Template.Spec.Containers)
	g.Expect(findEnv("GODEBUG", container.Env)).To(HaveField("Value", "fips140=only"))
	g.Expect(findEnv("GODEBUG", ss.Spec.Template.Spec.InitContainers[0].Env)).To(BeNil(),
		"GODEBUG is only set on the monitor container")

	instance.Annotations = nil
	ss, err = handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	container = monitorContainer(ss.Spec.Template.Spec.Containers)
	g.Expect(findEnv("GODEBUG", container.Env)).To(BeNil(), "GODEBUG should be pruned when disabled")
	g.Expect(findEnv("HOME", container.Env)).ToNot(BeNil())
}

func TestMonitorStatefulSetPaused(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)

	first, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	first.Annotations = map[string]string{annotations.PausedReconciliation: "true"}
	g.Expect(c.Update(t.Context(), first)).To(Succeed())

	instance.Spec.Monitoring.TLog.Interval = &metav1.Duration{Duration: 5 * time.Minute}
	ss, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	container := monitorContainer(ss.Spec.Template.Spec.Containers)
	g.Expect(container.Command[2]).To(ContainSubstring("--interval=10m0s"), "paused StatefulSet must not be updated")
}

func TestMonitorStatefulSetSecurityContexts(t *testing.T) {
	originalOpenShift := config.Openshift
	t.Cleanup(func() { config.Openshift = originalOpenShift })

	t.Run("Kubernetes", func(t *testing.T) {
		config.Openshift = false
		g := NewWithT(t)
		instance := createMonitorInstance()

		ss, err := handleStatefulSet(t, createMonitorClient(instance), instance)
		g.Expect(err).ToNot(HaveOccurred())

		podSpec := ss.Spec.Template.Spec
		g.Expect(podSpec.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()))
		g.Expect(podSpec.SecurityContext.FSGroup).To(HaveValue(Equal(int64(1001))))
		for _, c := range append(podSpec.InitContainers, podSpec.Containers...) {
			g.Expect(c.SecurityContext).ToNot(BeNil(), "container %s", c.Name)
			g.Expect(c.SecurityContext.RunAsNonRoot).To(HaveValue(BeTrue()), "container %s", c.Name)
			g.Expect(c.SecurityContext.AllowPrivilegeEscalation).To(HaveValue(BeFalse()), "container %s", c.Name)
			g.Expect(c.SecurityContext.Capabilities.Drop).To(ConsistOf(core.Capability("ALL")), "container %s", c.Name)
			g.Expect(c.SecurityContext.RunAsUser).To(HaveValue(Equal(int64(1001))), "container %s", c.Name)
		}
	})

	t.Run("OpenShift", func(t *testing.T) {
		config.Openshift = true
		g := NewWithT(t)
		instance := createMonitorInstance()

		ss, err := handleStatefulSet(t, createMonitorClient(instance), instance)
		g.Expect(err).ToNot(HaveOccurred())

		podSpec := ss.Spec.Template.Spec
		g.Expect(podSpec.SecurityContext.FSGroup).To(BeNil())
		for _, c := range append(podSpec.InitContainers, podSpec.Containers...) {
			g.Expect(c.SecurityContext.RunAsUser).To(BeNil(), "container %s", c.Name)
		}
	})
}

func TestMonitorStatefulSetLegacyMigration(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)
	legacy := createLegacy(t, c, instance)
	g.Expect(managers(legacy)).To(ConsistOf(kubernetes.LegacyFieldManager))

	migrated, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	// The API server defaults these on create; the fake client does not, so add
	// them to the legacy spec. Everything else must match exactly: a pod template
	// change would roll the monitor pod.
	expected := legacy.Spec.DeepCopy()
	expected.PodManagementPolicy = apps.OrderedReadyPodManagement
	expected.UpdateStrategy = apps.StatefulSetUpdateStrategy{
		Type:          apps.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &apps.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(0))},
	}
	g.Expect(migrated.Spec).To(Equal(*expected))
	g.Expect(managers(migrated)).To(ConsistOf(fieldManager))
}

func TestMonitorStatefulSetLegacyPrune(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)
	createLegacy(t, c, instance, func(ss *apps.StatefulSet) error {
		ss.Spec.Template.Spec.Containers[0].Env = append(ss.Spec.Template.Spec.Containers[0].Env,
			core.EnvVar{Name: "STALE", Value: "left-by-old-operator"})
		return nil
	})

	migrated, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	container := monitorContainer(migrated.Spec.Template.Spec.Containers)
	g.Expect(container.Env).To(ConsistOf(core.EnvVar{Name: "HOME", Value: mountPath}), "only the stale entry is removed")
}

func TestMonitorStatefulSetPreservesForeignFields(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := createMonitorClient(instance)
	created, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	// What `kubectl annotate` and `kubectl rollout restart` do.
	patch := client.MergeFrom(created.DeepCopy())
	created.Annotations = map[string]string{"example.com/owner": "other"}
	created.Spec.Template.Annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": "2026-10-26T00:00:00Z"}
	g.Expect(c.Patch(t.Context(), created, patch, client.FieldOwner("kubectl"))).To(Succeed())

	// Change the CR so the next apply actually updates the StatefulSet.
	instance.Spec.Monitoring.TLog.Interval = &metav1.Duration{Duration: 5 * time.Minute}
	updated, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(updated.Annotations).To(HaveKeyWithValue("example.com/owner", "other"))
	g.Expect(updated.Spec.Template.Annotations).To(HaveKeyWithValue("kubectl.kubernetes.io/restartedAt", "2026-10-26T00:00:00Z"))
	g.Expect(managers(updated)).To(ContainElements(fieldManager, "kubectl"))
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
	if err := c.Get(ctx, client.ObjectKeyFromObject(ss), ss); err != nil {
		t.Fatalf("getting legacy statefulset: %v", err)
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
