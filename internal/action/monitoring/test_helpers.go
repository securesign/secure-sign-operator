package monitoring

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// setupTest provides common test setup for all monitoring tests
func setupTest(_ *testing.T, apiAvailable bool, schemeAdders ...func(*runtime.Scheme) error) (client.Client, *record.FakeRecorder, logr.Logger, *ServiceMonitorRegistry, *runtime.Scheme) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	for _, adder := range schemeAdders {
		utilruntime.Must(adder(scheme))
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	recorder := record.NewFakeRecorder(100)
	logger := logr.Discard()

	registry := NewRegistry(c, recorder, logger)
	if apiAvailable {
		registry.SetAPIAvailable(true)
	}

	return c, recorder, logger, registry, scheme
}

// createTestSpec creates a ServiceMonitorSpec for testing purposes
func createTestSpec(namespace, name string) *ServiceMonitorSpec {
	return &ServiceMonitorSpec{
		OwnerKey: types.NamespacedName{
			Namespace: namespace,
			Name:      "test-owner",
		},
		OwnerGVK: schema.GroupVersionKind{
			Group:   "test.io",
			Version: "v1",
			Kind:    "TestResource",
		},
		Namespace: namespace,
		Name:      name,
		EnsureFuncs: []func(*unstructured.Unstructured) error{
			func(u *unstructured.Unstructured) error {
				return nil
			},
		},
	}
}

// createOwnerFromSpec creates a test owner object from a ServiceMonitorSpec
func createOwnerFromSpec(spec *ServiceMonitorSpec) *unstructured.Unstructured {
	owner := &unstructured.Unstructured{}
	owner.SetGroupVersionKind(spec.OwnerGVK)
	owner.SetNamespace(spec.OwnerKey.Namespace)
	owner.SetName(spec.OwnerKey.Name)
	owner.SetUID("test-uid")
	return owner
}

// mockInstance is a test implementation of ConditionsAwareObject
type mockInstance struct {
	*unstructured.Unstructured
	conditions []metav1.Condition
}

func (m *mockInstance) GetConditions() []metav1.Condition {
	return m.conditions
}

func (m *mockInstance) SetCondition(condition metav1.Condition) {
	meta.SetStatusCondition(&m.conditions, condition)
}

// newMockInstance creates a new mockInstance for testing
func newMockInstance(namespace, name string) *mockInstance {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "test.io",
		Version: "v1",
		Kind:    "MockResource",
	})
	u.SetNamespace(namespace)
	u.SetName(name)
	u.SetUID(types.UID("test-uid"))

	return &mockInstance{
		Unstructured: u,
		conditions:   []metav1.Condition{},
	}
}

// createMonitoringAction creates a configured monitoring action with dependencies injected
func createMonitoringAction(
	c client.Client,
	recorder *record.FakeRecorder,
	logger logr.Logger,
	registry *ServiceMonitorRegistry,
	isMonitoringEnabled func(*mockInstance) bool,
	customEndpointBuilder func(*mockInstance) []func(*unstructured.Unstructured) error,
) *genericMonitoringAction[*mockInstance] {
	config := MonitoringConfig[*mockInstance]{
		ComponentName:      "test-component",
		DeploymentName:     "test-deployment",
		MonitoringRoleName: "test-role",
		MetricsPortName:    "metrics",
		Registry:           registry,
		IsMonitoringEnabled: func(instance *mockInstance) bool {
			if isMonitoringEnabled != nil {
				return isMonitoringEnabled(instance)
			}
			return true
		},
		CustomEndpointBuilder: customEndpointBuilder,
	}

	action := NewMonitoringAction(config).(*genericMonitoringAction[*mockInstance])
	action.InjectClient(c)
	action.InjectLogger(logger)
	action.InjectRecorder(recorder)

	return action
}

// setupRegistryTest provides simplified setup for registry tests
func setupRegistryTest(t *testing.T) (client.Client, *ServiceMonitorRegistry) {
	c, _, _, registry, _ := setupTest(t, false)
	return c, registry
}

// createServiceMonitorCRD creates a test ServiceMonitor CRD for testing
func createServiceMonitorCRD(established bool) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: ServiceMonitorCRDName,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "monitoring.coreos.com",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     "ServiceMonitor",
				ListKind: "ServiceMonitorList",
				Plural:   "servicemonitors",
				Singular: "servicemonitor",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	if established {
		crd.Status.Conditions = []apiextensionsv1.CustomResourceDefinitionCondition{
			{
				Type:   apiextensionsv1.Established,
				Status: apiextensionsv1.ConditionTrue,
			},
		}
	}

	return crd
}

// setupCRDWatcherTest provides setup for CRD watcher tests
func setupCRDWatcherTest(t *testing.T) *CRDWatcherReconciler {
	g := NewWithT(t)

	c, _, _, registry, scheme := setupTest(t, false, apiextensionsv1.AddToScheme)

	reconciler, err := NewCRDWatcher(c, scheme, registry)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(reconciler).ToNot(BeNil())

	return reconciler
}

// setupMonitoringActionTest combines setupTest and createMonitoringAction for common test setup
func setupMonitoringActionTest(t *testing.T, apiAvailable bool) (client.Client, *record.FakeRecorder, logr.Logger, *ServiceMonitorRegistry, *genericMonitoringAction[*mockInstance]) {
	c, recorder, logger, registry, _ := setupTest(t, apiAvailable)
	action := createMonitoringAction(c, recorder, logger, registry, nil, nil)
	return c, recorder, logger, registry, action
}
