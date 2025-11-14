package monitoring

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGenericMonitoringActionCanHandle(t *testing.T) {
	tests := []struct {
		name              string
		reason            string
		monitoringEnabled bool
		expectedCanHandle bool
	}{
		{
			name:              "can handle - Creating state with monitoring enabled",
			reason:            constants.Creating,
			monitoringEnabled: true,
			expectedCanHandle: true,
		},
		{
			name:              "can handle - Ready state with monitoring enabled",
			reason:            constants.Ready,
			monitoringEnabled: true,
			expectedCanHandle: true,
		},
		{
			name:              "cannot handle - Creating state with monitoring disabled",
			reason:            constants.Creating,
			monitoringEnabled: false,
			expectedCanHandle: false,
		},
		{
			name:              "cannot handle - Ready state with monitoring disabled",
			reason:            constants.Ready,
			monitoringEnabled: false,
			expectedCanHandle: false,
		},
		{
			name:              "cannot handle - other state",
			reason:            constants.Failure,
			monitoringEnabled: true,
			expectedCanHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			c, recorder, logger, _, _ := setupTest(t, true)

			action := createMonitoringAction(c, recorder, logger, nil, func(instance *mockInstance) bool {
				return tt.monitoringEnabled
			}, nil)

			instance := newMockInstance("default", "test-instance")
			instance.SetCondition(metav1.Condition{
				Type:   constants.Ready,
				Reason: tt.reason,
				Status: metav1.ConditionTrue,
			})

			result := action.CanHandle(ctx, instance)
			g.Expect(result).To(Equal(tt.expectedCanHandle))
		})
	}
}

func TestGenericMonitoringActionHandle(t *testing.T) {
	tests := []struct {
		name                         string
		apiAvailable                 bool
		customEndpointBuilder        func(*mockInstance) []func(*unstructured.Unstructured) error
		instances                    []*mockInstance
		expectedSpecCount            int
		expectedContinue             bool
		validateCustomEndpointCalled bool
		customEndpointCalled         *bool
		validateSpec                 func(*testing.T, *ServiceMonitorRegistry)
	}{
		{
			name:         "basic handle",
			apiAvailable: true,
			instances: []*mockInstance{
				createTestInstance("default", "test-instance"),
			},
			expectedSpecCount: 1,
			expectedContinue:  true,
			validateSpec: func(t *testing.T, registry *ServiceMonitorRegistry) {
				g := NewWithT(t)
				specs := registry.GetAll()
				found := false
				for _, spec := range specs {
					if spec.Name == "test-deployment" && spec.Namespace == "default" {
						found = true
						g.Expect(spec.OwnerKey.Name).To(Equal("test-instance"))
						g.Expect(spec.OwnerKey.Namespace).To(Equal("default"))
						g.Expect(spec.EnsureFuncs).ToNot(BeEmpty())
						break
					}
				}
				g.Expect(found).To(BeTrue(), "ServiceMonitor spec should be registered")
			},
		},
		{
			name:         "handle with custom endpoint builder",
			apiAvailable: true,
			instances: []*mockInstance{
				createTestInstance("default", "test-instance"),
			},
			customEndpointBuilder: func(instance *mockInstance) []func(*unstructured.Unstructured) error {
				return []func(*unstructured.Unstructured) error{
					func(u *unstructured.Unstructured) error {
						return nil
					},
				}
			},
			expectedSpecCount:            1,
			expectedContinue:             true,
			validateCustomEndpointCalled: true,
		},
		{
			name:         "handle with API unavailable",
			apiAvailable: false,
			instances: []*mockInstance{
				createTestInstance("default", "test-instance"),
			},
			expectedSpecCount: 1,
			expectedContinue:  true,
		},
		{
			name:         "handle multiple instances",
			apiAvailable: true,
			instances: []*mockInstance{
				createTestInstance("default", "test-instance-1"),
				createTestInstance("other-ns", "test-instance-2"),
			},
			expectedSpecCount: 2,
			expectedContinue:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			c, recorder, logger, registry, _ := setupTest(t, tt.apiAvailable)

			var monitoringAction *genericMonitoringAction[*mockInstance]
			customEndpointBuilderCalled := false

			if tt.customEndpointBuilder != nil {
				wrappedBuilder := func(instance *mockInstance) []func(*unstructured.Unstructured) error {
					customEndpointBuilderCalled = true
					return tt.customEndpointBuilder(instance)
				}
				monitoringAction = createMonitoringAction(c, recorder, logger, registry, nil, wrappedBuilder)
			} else {
				monitoringAction = createMonitoringAction(c, recorder, logger, registry, nil, nil)
			}

			for _, instance := range tt.instances {
				g.Expect(c.Create(ctx, instance.Unstructured)).To(Succeed())
				result := monitoringAction.Handle(ctx, instance)
				g.Expect(action.IsContinue(result)).To(Equal(tt.expectedContinue))
			}

			specs := registry.GetAll()
			g.Expect(specs).To(HaveLen(tt.expectedSpecCount))

			if tt.validateSpec != nil {
				tt.validateSpec(t, registry)
			}

			if tt.validateCustomEndpointCalled {
				g.Expect(customEndpointBuilderCalled).To(BeTrue())
			}
		})
	}
}

func TestGenericMonitoringActionIntegration(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()

	c, _, _, registry, monitoringAction := setupMonitoringActionTest(t, true)

	instance := newMockInstance("default", "test-instance")
	instance.SetCondition(metav1.Condition{
		Type:   constants.Ready,
		Reason: constants.Creating,
		Status: metav1.ConditionTrue,
	})

	g.Expect(c.Create(ctx, instance.Unstructured)).To(Succeed())

	g.Expect(monitoringAction.CanHandle(ctx, instance)).To(BeTrue())

	result := monitoringAction.Handle(ctx, instance)
	g.Expect(action.IsContinue(result)).To(BeTrue())

	specs := registry.GetAll()
	g.Expect(specs).To(HaveLen(1))

	spec := specs[0]
	g.Expect(spec.Name).To(Equal("test-deployment"))
	g.Expect(spec.Namespace).To(Equal("default"))
	g.Expect(spec.OwnerKey.Name).To(Equal("test-instance"))
	g.Expect(spec.OwnerKey.Namespace).To(Equal("default"))
	g.Expect(spec.EnsureFuncs).ToNot(BeEmpty())
}

// createTestInstance is a helper to create a test instance with default conditions
func createTestInstance(namespace, name string) *mockInstance {
	instance := newMockInstance(namespace, name)
	instance.SetCondition(metav1.Condition{
		Type:   constants.Ready,
		Reason: constants.Creating,
		Status: metav1.ConditionTrue,
	})
	return instance
}
