package monitor

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tufURL       = "http://tuf.default.svc"
	ctlogURL     = "http://ctlog.default.svc/trusted-artifact-signer"
	fieldManager = kubernetes.FieldManager + "/ctlog-controller"
)

func createMonitorInstance() *rhtasv1.CTlog {
	return &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ctlog",
			Namespace: "default",
			UID:       "ctlog-uid",
		},
		Spec: rhtasv1.CTlogSpec{
			Monitoring: rhtasv1.MonitoringWithTLogConfig{
				TLog: rhtasv1.TlogMonitoring{
					Enabled:  ptr.To(true),
					Interval: &metav1.Duration{Duration: 10 * time.Minute},
				},
				Tuf: rhtasv1.ServiceReference{URL: tufURL},
			},
		},
		Status: rhtasv1.CTlogStatus{
			URL: ctlogURL,
			Conditions: []metav1.Condition{{
				Type:   constants.ReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: state.Creating.String(),
			}},
		},
	}
}

func handleStatefulSet(t *testing.T, c client.Client, instance *rhtasv1.CTlog) (*apps.StatefulSet, error) {
	t.Helper()

	result := testAction.PrepareAction(kubernetes.WithFieldManager(c, fieldManager), NewStatefulSetAction()).Handle(t.Context(), instance)
	if result != nil && result.Err != nil {
		return nil, result.Err
	}

	ss := &apps.StatefulSet{}
	if err := c.Get(t.Context(), client.ObjectKey{Name: actions.MonitorStatefulSetName, Namespace: instance.Namespace}, ss); err != nil {
		return nil, err
	}
	return ss, nil
}

func findContainer(containers []core.Container, name string) *core.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

func findEnv(envs []core.EnvVar, name string) *core.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

func TestMonitorStatefulSetCreatesFunctionalMonitor(t *testing.T) {
	proxyEnv := []core.EnvVar{
		{Name: "HTTP_PROXY", Value: "http://proxy.example.com:3128"},
		{Name: "HTTPS_PROXY", Value: "https://proxy.example.com:3128"},
		{Name: "NO_PROXY", Value: "localhost,127.0.0.1"},
	}
	for _, env := range proxyEnv {
		t.Setenv(env.Name, env.Value)
	}

	g := NewWithT(t)
	instance := createMonitorInstance()
	instance.Spec.TrustedCA = &rhtasv1.LocalObjectReference{Name: "trusted-ca"}
	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	ss, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	expectedLabels := labels.For(actions.MonitorComponentName, actions.MonitorStatefulSetName, instance.Name)
	g.Expect(ss.Labels).To(Equal(expectedLabels))
	g.Expect(ss.OwnerReferences).To(HaveLen(1))
	g.Expect(ss.OwnerReferences[0].Name).To(Equal(instance.Name))
	g.Expect(ss.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))
	g.Expect(ss.Spec.Replicas).To(HaveValue(Equal(int32(1))))
	g.Expect(ss.Spec.PodManagementPolicy).To(Equal(apps.OrderedReadyPodManagement))
	g.Expect(ss.Spec.UpdateStrategy.Type).To(Equal(apps.RollingUpdateStatefulSetStrategyType))
	g.Expect(ss.Spec.UpdateStrategy.RollingUpdate).To(HaveField("Partition", HaveValue(Equal(int32(0)))))
	g.Expect(ss.Spec.Selector.MatchLabels).To(Equal(expectedLabels))
	g.Expect(ss.Spec.Template.Labels).To(Equal(expectedLabels))
	g.Expect(ss.Spec.Template.Spec.ServiceAccountName).To(Equal(actions.RBACMonitorName))

	g.Expect(ss.Spec.VolumeClaimTemplates).To(HaveLen(1))
	storage := ss.Spec.VolumeClaimTemplates[0]
	g.Expect(storage.Name).To(Equal(storageVolumeName))
	g.Expect(storage.Spec.AccessModes).To(ConsistOf(core.ReadWriteOnce))
	g.Expect(storage.Spec.Resources.Requests.Storage().String()).To(Equal("5Mi"))

	container := findContainer(ss.Spec.Template.Spec.Containers, actions.MonitorStatefulSetName)
	g.Expect(container).ToNot(BeNil())
	g.Expect(container.Image).To(Equal(images.Registry.Get(images.CTLogMonitor)))
	g.Expect(container.Command).To(Equal([]string{
		"/bin/sh",
		"-c",
		fmt.Sprintf(
			`/ctlog_monitor --file=%s/checkpoint_log.txt --once=false --interval=10m0s --url=%s --tuf-repository=%s --tuf-root-path="%s/root.json"`,
			mountPath, ctlogURL, tufURL, mountPath),
	}))
	g.Expect(container.Ports).To(ConsistOf(core.ContainerPort{
		Name:          actions.MetricsPortName,
		ContainerPort: actions.MonitorMetricsPort,
		Protocol:      core.ProtocolTCP,
	}))
	g.Expect(findEnv(container.Env, "HOME")).To(HaveField("Value", mountPath))
	g.Expect(container.VolumeMounts).To(ContainElement(core.VolumeMount{
		Name:      storageVolumeName,
		MountPath: mountPath,
	}))
	g.Expect(ss.Spec.Template.Spec.Volumes).To(ContainElement(And(
		HaveField("Name", "ca-trust"),
		HaveField("Projected.Sources", ContainElement(HaveField("ConfigMap.Name", "trusted-ca"))),
	)))
	g.Expect(container.VolumeMounts).To(ContainElement(HaveField("Name", "ca-trust")))
	g.Expect(findEnv(container.Env, "SSL_CERT_DIR")).ToNot(BeNil())

	initContainer := findContainer(ss.Spec.Template.Spec.InitContainers, initContainerName)
	g.Expect(initContainer).ToNot(BeNil())
	g.Expect(initContainer.Env).To(ContainElements(proxyEnv), "tuf-init needs the proxy to reach the CTlog ingress")
	g.Expect(initContainer.SecurityContext).To(And(
		HaveField("RunAsNonRoot", HaveValue(BeTrue())),
		HaveField("AllowPrivilegeEscalation", HaveValue(BeFalse())),
		HaveField("Capabilities.Drop", ConsistOf(core.Capability("ALL"))),
	), "tuf-init needs the container security context")
	g.Expect(initContainer.Image).To(Equal(images.Registry.Get(images.CTLogMonitor)))
	g.Expect(initContainer.Command[2]).To(ContainSubstring(ctlogURL + "/ct/v1/get-sth"))
	g.Expect(initContainer.Command[2]).To(ContainSubstring(tufURL + "/root.json"))
	g.Expect(findEnv(initContainer.Env, "SSL_CERT_DIR")).To(BeNil())
	g.Expect(initContainer.VolumeMounts).To(ConsistOf(core.VolumeMount{
		Name:      storageVolumeName,
		MountPath: mountPath,
	}))
}

func TestMonitorStatefulSetUpdatesMonitoringInterval(t *testing.T) {
	g := NewWithT(t)
	instance := createMonitorInstance()
	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

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

	container := findContainer(updated.Spec.Template.Spec.Containers, actions.MonitorStatefulSetName)
	g.Expect(container).ToNot(BeNil())
	g.Expect(container.Command[2]).To(ContainSubstring("--interval=5m0s"))
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, actions.MonitorCondition).Reason).
		To(Equal(state.Creating.String()))
}

func TestMonitorStatefulSetPrunesGODEBUGAndPreservesForeignFields(t *testing.T) {
	t.Setenv("GODEBUG", "")
	g := NewWithT(t)
	instance := createMonitorInstance()
	instance.Annotations = map[string]string{annotations.Godebug: "fips140=only"}
	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	ss, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	container := findContainer(ss.Spec.Template.Spec.Containers, actions.MonitorStatefulSetName)
	g.Expect(container).To(HaveField("Env", ContainElement(core.EnvVar{Name: "GODEBUG", Value: "fips140=only"})))
	init := findContainer(ss.Spec.Template.Spec.InitContainers, initContainerName)
	g.Expect(init).To(HaveField("Env", Not(ContainElement(HaveField("Name", "GODEBUG")))))

	patch := client.MergeFrom(ss.DeepCopy())
	ss.Spec.Template.Annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": "2026-10-26T00:00:00Z"}
	g.Expect(c.Patch(t.Context(), ss, patch, client.FieldOwner("kubectl"))).To(Succeed())

	delete(instance.Annotations, annotations.Godebug)
	updated, err := handleStatefulSet(t, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	container = findContainer(updated.Spec.Template.Spec.Containers, actions.MonitorStatefulSetName)
	g.Expect(container).To(HaveField("Env", Not(ContainElement(HaveField("Name", "GODEBUG")))))
	g.Expect(container).To(HaveField("Env", ContainElement(core.EnvVar{Name: "HOME", Value: mountPath})))
	g.Expect(updated.Spec.Template.Annotations).To(Equal(ss.Spec.Template.Annotations))
}
