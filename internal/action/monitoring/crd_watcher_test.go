package monitoring

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func TestNewCRDWatcher(t *testing.T) {
	g := NewWithT(t)

	c, _, _, registry, scheme := setupTest(t, false, apiextensionsv1.AddToScheme)

	reconciler, err := NewCRDWatcher(c, scheme, registry)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(reconciler).ToNot(BeNil())
	g.Expect(reconciler.Client).To(Equal(c))
	g.Expect(reconciler.Scheme).To(Equal(scheme))
	g.Expect(reconciler.Registry).To(Equal(registry))
}

func TestReconcileServiceMonitorCRD(t *testing.T) {
	tests := []struct {
		name                 string
		crdName              string
		createCRD            bool
		crdEstablished       bool
		expectedAPIAvailable *bool // nil means don't check
	}{
		{
			name:                 "non-ServiceMonitor CRD",
			crdName:              "othercrd.example.com",
			createCRD:            false,
			expectedAPIAvailable: nil, // Don't check for non-ServiceMonitor CRDs
		},
		{
			name:                 "ServiceMonitor CRD established",
			crdName:              ServiceMonitorCRDName,
			createCRD:            true,
			crdEstablished:       true,
			expectedAPIAvailable: boolPtr(true),
		},
		{
			name:                 "ServiceMonitor CRD not established",
			crdName:              ServiceMonitorCRDName,
			createCRD:            true,
			crdEstablished:       false,
			expectedAPIAvailable: boolPtr(false),
		},
		{
			name:                 "ServiceMonitor CRD not found",
			crdName:              ServiceMonitorCRDName,
			createCRD:            false,
			expectedAPIAvailable: boolPtr(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()

			reconciler := setupCRDWatcherTest(t)

			if tt.createCRD {
				crd := createServiceMonitorCRD(tt.crdEstablished)
				g.Expect(reconciler.Client.Create(ctx, crd)).To(Succeed())
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name: tt.crdName,
				},
			}

			result, err := reconciler.Reconcile(ctx, req)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(ctrl.Result{}))

			if tt.expectedAPIAvailable != nil {
				g.Expect(reconciler.Registry.IsAPIAvailable()).To(Equal(*tt.expectedAPIAvailable))
			}
		})
	}
}

func TestReconcileServiceMonitorCRDTransitions(t *testing.T) {
	g := NewWithT(t)
	ctx := context.TODO()

	reconciler := setupCRDWatcherTest(t)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: ServiceMonitorCRDName,
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(reconciler.Registry.IsAPIAvailable()).To(BeFalse())

	crd := createServiceMonitorCRD(true)
	g.Expect(reconciler.Client.Create(ctx, crd)).To(Succeed())

	result, err = reconciler.Reconcile(ctx, req)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(reconciler.Registry.IsAPIAvailable()).To(BeTrue())

	g.Expect(reconciler.Client.Delete(ctx, crd)).To(Succeed())

	result, err = reconciler.Reconcile(ctx, req)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(reconciler.Registry.IsAPIAvailable()).To(BeFalse())
}

func TestIsCRDEstablished(t *testing.T) {
	tests := []struct {
		name        string
		conditions  []apiextensionsv1.CustomResourceDefinitionCondition
		established bool
	}{
		{
			name: "established",
			conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:   apiextensionsv1.Established,
					Status: apiextensionsv1.ConditionTrue,
				},
			},
			established: true,
		},
		{
			name: "not established",
			conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:   apiextensionsv1.Established,
					Status: apiextensionsv1.ConditionFalse,
				},
			},
			established: false,
		},
		{
			name:        "no conditions",
			conditions:  []apiextensionsv1.CustomResourceDefinitionCondition{},
			established: false,
		},
		{
			name: "other conditions only",
			conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
				{
					Type:   "NamesAccepted",
					Status: apiextensionsv1.ConditionTrue,
				},
			},
			established: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			crd := &apiextensionsv1.CustomResourceDefinition{
				Status: apiextensionsv1.CustomResourceDefinitionStatus{
					Conditions: tt.conditions,
				},
			}

			result := isCRDEstablished(crd)
			g.Expect(result).To(Equal(tt.established))
		})
	}
}

func TestServiceMonitorCRDPredicate(t *testing.T) {
	pred := serviceMonitorCRDPredicate()

	serviceMonitorCRD := createServiceMonitorCRD(true)
	otherCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-crd.example.com",
		},
	}

	tests := []struct {
		name     string
		testFunc func(predicate.Predicate) bool
	}{
		{
			name: "CreateFunc - ServiceMonitor CRD",
			testFunc: func(p predicate.Predicate) bool {
				return p.Create(event.CreateEvent{Object: serviceMonitorCRD})
			},
		},
		{
			name: "CreateFunc - Other CRD",
			testFunc: func(p predicate.Predicate) bool {
				return !p.Create(event.CreateEvent{Object: otherCRD})
			},
		},
		{
			name: "UpdateFunc - ServiceMonitor CRD",
			testFunc: func(p predicate.Predicate) bool {
				return p.Update(event.UpdateEvent{ObjectNew: serviceMonitorCRD})
			},
		},
		{
			name: "UpdateFunc - Other CRD",
			testFunc: func(p predicate.Predicate) bool {
				return !p.Update(event.UpdateEvent{ObjectNew: otherCRD})
			},
		},
		{
			name: "DeleteFunc - ServiceMonitor CRD",
			testFunc: func(p predicate.Predicate) bool {
				return p.Delete(event.DeleteEvent{Object: serviceMonitorCRD})
			},
		},
		{
			name: "DeleteFunc - Other CRD",
			testFunc: func(p predicate.Predicate) bool {
				return !p.Delete(event.DeleteEvent{Object: otherCRD})
			},
		},
		{
			name: "GenericFunc - ServiceMonitor CRD",
			testFunc: func(p predicate.Predicate) bool {
				return p.Generic(event.GenericEvent{Object: serviceMonitorCRD})
			},
		},
		{
			name: "GenericFunc - Other CRD",
			testFunc: func(p predicate.Predicate) bool {
				return !p.Generic(event.GenericEvent{Object: otherCRD})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.testFunc(pred)
			if !result {
				t.Errorf("Predicate test failed for %s", tt.name)
			}
		})
	}
}

// boolPtr helper for creating *bool values
func boolPtr(b bool) *bool {
	return &b
}
