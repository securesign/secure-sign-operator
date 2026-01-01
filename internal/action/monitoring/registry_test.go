package monitoring

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewRegistry(t *testing.T) {
	g := NewWithT(t)

	c, recorder, _, registry, _ := setupTest(t, false)

	g.Expect(registry).ToNot(BeNil())
	g.Expect(registry.specs).ToNot(BeNil())
	g.Expect(registry.notifiedOwners).ToNot(BeNil())
	g.Expect(registry.client).To(Equal(c))
	g.Expect(registry.recorder).To(Equal(recorder))
	g.Expect(registry.apiAvailable).To(BeFalse())
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name          string
		apiAvailable  bool
		expectedEvent string
		isNewOwner    bool
	}{
		{
			name:          "register with API available",
			apiAvailable:  true,
			expectedEvent: "ServiceMonitorAPIAvailable",
			isNewOwner:    true,
		},
		{
			name:          "register with API unavailable",
			apiAvailable:  false,
			expectedEvent: "ServiceMonitorAPIUnavailable",
			isNewOwner:    true,
		},
		{
			name:          "register existing owner",
			apiAvailable:  true,
			expectedEvent: "",
			isNewOwner:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			c, registry := setupRegistryTest(t)
			registry.SetAPIAvailable(tt.apiAvailable)

			spec := createTestSpec("default", "test-monitor")

			owner := createOwnerFromSpec(spec)

			g.Expect(c.Create(ctx, owner)).To(Succeed())

			registry.Register(ctx, spec, owner)

			g.Expect(registry.specs).To(HaveKey(types.NamespacedName{
				Namespace: spec.Namespace,
				Name:      spec.Name,
			}))

			if !tt.isNewOwner {
				registry.Register(ctx, spec, owner)
			}
		})
	}
}

func TestRegisterSpec(t *testing.T) {
	tests := []struct {
		name                  string
		apiAvailable          bool
		registerTwice         bool
		expectedIsNewOwner    bool
		expectedAPIAvailable  bool
		expectedSpecCount     int
		expectedNotifiedCount int
	}{
		{
			name:                  "new owner with API available",
			apiAvailable:          true,
			registerTwice:         false,
			expectedIsNewOwner:    true,
			expectedAPIAvailable:  true,
			expectedSpecCount:     1,
			expectedNotifiedCount: 1,
		},
		{
			name:                  "new owner with API unavailable",
			apiAvailable:          false,
			registerTwice:         false,
			expectedIsNewOwner:    true,
			expectedAPIAvailable:  false,
			expectedSpecCount:     1,
			expectedNotifiedCount: 1,
		},
		{
			name:                  "existing owner with API available",
			apiAvailable:          true,
			registerTwice:         true,
			expectedIsNewOwner:    false,
			expectedAPIAvailable:  true,
			expectedSpecCount:     1,
			expectedNotifiedCount: 1,
		},
		{
			name:                  "existing owner with API unavailable",
			apiAvailable:          false,
			registerTwice:         true,
			expectedIsNewOwner:    false,
			expectedAPIAvailable:  false,
			expectedSpecCount:     1,
			expectedNotifiedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			_, registry := setupRegistryTest(t)
			registry.SetAPIAvailable(tt.apiAvailable)

			spec := createTestSpec("default", "test-monitor")

			if tt.registerTwice {
				registry.registerSpec(spec)
			}

			isNewOwner, apiAvailable := registry.registerSpec(spec)

			g.Expect(isNewOwner).To(Equal(tt.expectedIsNewOwner))
			g.Expect(apiAvailable).To(Equal(tt.expectedAPIAvailable))
			g.Expect(registry.specs).To(HaveLen(tt.expectedSpecCount))
			g.Expect(registry.notifiedOwners).To(HaveLen(tt.expectedNotifiedCount))
		})
	}
}

func TestRegisterSpecOwnerChange(t *testing.T) {
	g := NewWithT(t)

	_, registry := setupRegistryTest(t)

	spec1 := createTestSpec("default", "test-monitor")

	isNewOwner, _ := registry.registerSpec(spec1)
	g.Expect(isNewOwner).To(BeTrue())
	g.Expect(registry.notifiedOwners).To(HaveLen(1))

	spec2 := createTestSpec("default", "test-monitor")
	spec2.OwnerKey.Name = "differentowner"

	isNewOwner, _ = registry.registerSpec(spec2)

	g.Expect(isNewOwner).To(BeTrue())
	g.Expect(registry.specs).To(HaveLen(1))
	g.Expect(registry.notifiedOwners).To(HaveLen(1))
}

func TestGetAll(t *testing.T) {
	g := NewWithT(t)

	_, registry := setupRegistryTest(t)

	spec1 := createTestSpec("default", "monitor-1")
	spec2 := createTestSpec("default", "monitor-2")
	spec3 := createTestSpec("other-ns", "monitor-3")

	registry.registerSpec(spec1)
	registry.registerSpec(spec2)
	registry.registerSpec(spec3)

	specs := registry.GetAll()

	g.Expect(specs).To(HaveLen(3))
}

func TestSetAPIAvailable(t *testing.T) {
	tests := []struct {
		name          string
		initialState  bool
		setValue      bool
		expectedValue bool
	}{
		{
			name:          "set to true from initial false",
			initialState:  false,
			setValue:      true,
			expectedValue: true,
		},
		{
			name:          "set to false from true",
			initialState:  true,
			setValue:      false,
			expectedValue: false,
		},
		{
			name:          "set to true when already true",
			initialState:  true,
			setValue:      true,
			expectedValue: true,
		},
		{
			name:          "set to false when already false",
			initialState:  false,
			setValue:      false,
			expectedValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			_, registry := setupRegistryTest(t)

			if tt.initialState {
				registry.SetAPIAvailable(true)
			}

			registry.SetAPIAvailable(tt.setValue)
			g.Expect(registry.IsAPIAvailable()).To(Equal(tt.expectedValue))
		})
	}
}

func TestReconcileAPIUnavailable(t *testing.T) {
	tests := []struct {
		name         string
		reconcileAll bool
		registerSpec bool
	}{
		{
			name:         "ReconcileAll with API unavailable",
			reconcileAll: true,
			registerSpec: true,
		},
		{
			name:         "ReconcileOne with API unavailable",
			reconcileAll: false,
			registerSpec: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			_, registry := setupRegistryTest(t)
			registry.SetAPIAvailable(false)

			spec := createTestSpec("default", "test-monitor")

			if tt.registerSpec {
				registry.registerSpec(spec)
			}

			var err error
			if tt.reconcileAll {
				err = registry.ReconcileAll(ctx)
			} else {
				err = registry.ReconcileOne(ctx, spec)
			}

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring("ServiceMonitor API not available"))
		})
	}
}

func TestGetOwnerObject(t *testing.T) {
	tests := []struct {
		name          string
		createOwner   bool
		registerSpec  bool
		expectError   bool
		expectCleanup bool
	}{
		{
			name:          "owner exists",
			createOwner:   true,
			registerSpec:  false,
			expectError:   false,
			expectCleanup: false,
		},
		{
			name:          "owner not found triggers cleanup",
			createOwner:   false,
			registerSpec:  true,
			expectError:   true,
			expectCleanup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			c, registry := setupRegistryTest(t)

			spec := createTestSpec("default", "test-monitor")

			if tt.createOwner {
				owner := createOwnerFromSpec(spec)
				g.Expect(c.Create(ctx, owner)).To(Succeed())
			}

			if tt.registerSpec {
				registry.registerSpec(spec)
			}

			retrievedOwner, err := registry.getOwnerObject(ctx, spec)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(retrievedOwner).To(BeNil())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(retrievedOwner).ToNot(BeNil())
				g.Expect(retrievedOwner.GetName()).To(Equal(spec.OwnerKey.Name))
				g.Expect(retrievedOwner.GetNamespace()).To(Equal(spec.OwnerKey.Namespace))
			}

			if tt.expectCleanup {
				g.Expect(registry.specs).To(BeEmpty())
				g.Expect(registry.notifiedOwners).To(BeEmpty())
			}
		})
	}
}

func TestCleanupOwner(t *testing.T) {
	g := NewWithT(t)

	_, registry := setupRegistryTest(t)

	spec := createTestSpec("default", "test-monitor")

	registry.registerSpec(spec)

	g.Expect(registry.specs).To(HaveLen(1))
	g.Expect(registry.notifiedOwners).To(HaveLen(1))

	registry.cleanupOwner(spec)

	g.Expect(registry.specs).To(BeEmpty())
	g.Expect(registry.notifiedOwners).To(BeEmpty())
}

func TestEmitEventToOwners(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()

	c, recorder, _, registry, _ := setupTest(t, false)

	spec1 := createTestSpec("default", "monitor-1")
	spec2 := createTestSpec("default", "monitor-2")
	spec2.OwnerKey.Name = "test-owner-2"

	owner1 := createOwnerFromSpec(spec1)

	owner2 := createOwnerFromSpec(spec2)

	g.Expect(c.Create(ctx, owner1)).To(Succeed())
	g.Expect(c.Create(ctx, owner2)).To(Succeed())

	registry.registerSpec(spec1)
	registry.registerSpec(spec2)

	registry.EmitEventToOwners(ctx, "Normal", "TestReason", "Test message")

	events := recorder.Events
	g.Expect(events).To(HaveLen(2), "Should emit events to both owners")

	for range 2 {
		event := <-events
		g.Expect(event).To(ContainSubstring("TestReason"))
		g.Expect(event).To(ContainSubstring("Test message"))
	}
}

func TestReconcileAllWithServiceMonitors(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()

	c, registry := setupRegistryTest(t)
	registry.SetAPIAvailable(true)

	serviceMonitorGVK := schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}

	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	sm.SetNamespace("default")
	sm.SetName("existing-monitor")

	g.Expect(c.Create(ctx, sm)).To(Succeed())

	spec := createTestSpec("default", "test-monitor")
	owner := createOwnerFromSpec(spec)

	g.Expect(c.Create(ctx, owner)).To(Succeed())

	spec.EnsureFuncs = []func(*unstructured.Unstructured) error{
		func(u *unstructured.Unstructured) error {
			u.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: spec.OwnerGVK.GroupVersion().String(),
					Kind:       spec.OwnerGVK.Kind,
					Name:       owner.GetName(),
					UID:        owner.GetUID(),
				},
			})
			return nil
		},
	}

	registry.registerSpec(spec)

	err := registry.ReconcileAll(ctx)

	g.Expect(err).ToNot(HaveOccurred())

	retrievedSM := &unstructured.Unstructured{}
	retrievedSM.SetGroupVersionKind(serviceMonitorGVK)

	err = c.Get(ctx, types.NamespacedName{
		Namespace: spec.Namespace,
		Name:      spec.Name,
	}, retrievedSM)

	g.Expect(err).ToNot(HaveOccurred(), "ServiceMonitor should be created")
	g.Expect(retrievedSM.GetName()).To(Equal(spec.Name))
	g.Expect(retrievedSM.GetNamespace()).To(Equal(spec.Namespace))

	ownerRefs := retrievedSM.GetOwnerReferences()
	g.Expect(ownerRefs).To(HaveLen(1), "ServiceMonitor should have one owner reference")
	g.Expect(ownerRefs[0].APIVersion).To(Equal(spec.OwnerGVK.GroupVersion().String()))
	g.Expect(ownerRefs[0].Kind).To(Equal(spec.OwnerGVK.Kind))
	g.Expect(ownerRefs[0].Name).To(Equal(owner.GetName()))
	g.Expect(ownerRefs[0].UID).To(Equal(owner.GetUID()))

	existingSM := &unstructured.Unstructured{}
	existingSM.SetGroupVersionKind(serviceMonitorGVK)
	err = c.Get(ctx, types.NamespacedName{
		Namespace: "default",
		Name:      "existing-monitor",
	}, existingSM)
	g.Expect(err).ToNot(HaveOccurred(), "Existing ServiceMonitor should still exist")

	g.Expect(registry.specs).To(HaveLen(1), "Registry should still contain the registered spec")
}
