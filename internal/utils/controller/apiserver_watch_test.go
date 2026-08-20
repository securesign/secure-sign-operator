package controller

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/config"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

func TestEnqueueAllOnClusterConfigChange(t *testing.T) {
	// The triggering object is the cluster APIServer singleton; it is ignored and
	// every instance of the watched CR kind must be enqueued regardless of namespace.
	apiServer := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	tests := []struct {
		name     string
		fulcios  []client.Object
		expected []types.NamespacedName
	}{
		{
			name:     "no instances enqueues nothing",
			fulcios:  nil,
			expected: nil,
		},
		{
			name: "single instance enqueued",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"}},
			},
			expected: []types.NamespacedName{{Name: "f1", Namespace: "ns1"}},
		},
		{
			name: "instances across namespaces are all enqueued",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"}},
				&rhtasv1.Fulcio{ObjectMeta: metav1.ObjectMeta{Name: "f2", Namespace: "ns2"}},
			},
			expected: []types.NamespacedName{
				{Name: "f1", Namespace: "ns1"},
				{Name: "f2", Namespace: "ns2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().WithObjects(tt.fulcios...).Build()

			mapFn := EnqueueAllOnClusterConfigChange(c, &rhtasv1.FulcioList{})
			got := mapFn(t.Context(), apiServer)

			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d requests, got %d: %v", len(tt.expected), len(got), got)
			}
			set := make(map[types.NamespacedName]bool, len(got))
			for _, r := range got {
				set[r.NamespacedName] = true
			}
			for _, want := range tt.expected {
				if !set[want] {
					t.Errorf("expected request %v not found in %v", want, got)
				}
			}
		})
	}
}

// spyRegistrar records Watches invocations so the OpenShift guard in
// WatchAPIServer can be asserted without standing up a manager.
type spyRegistrar struct {
	calls   int
	watched client.Object
}

func (s *spyRegistrar) Watches(object client.Object, _ handler.EventHandler, _ ...builder.WatchesOption) *builder.Builder {
	s.calls++
	s.watched = object
	return nil
}

func TestWatchAPIServer_OpenShiftGuard(t *testing.T) {
	c := testAction.FakeClientBuilder().Build()

	t.Run("not registered on vanilla kubernetes", func(t *testing.T) {
		orig := config.Openshift
		config.Openshift = false
		defer func() { config.Openshift = orig }()

		spy := &spyRegistrar{}
		WatchAPIServer(spy, c, &rhtasv1.FulcioList{})

		if spy.calls != 0 {
			t.Fatalf("expected no APIServer watch on vanilla k8s, got %d", spy.calls)
		}
	})

	t.Run("registered on OpenShift", func(t *testing.T) {
		orig := config.Openshift
		config.Openshift = true
		defer func() { config.Openshift = orig }()

		spy := &spyRegistrar{}
		WatchAPIServer(spy, c, &rhtasv1.FulcioList{})

		if spy.calls != 1 {
			t.Fatalf("expected exactly one APIServer watch on OpenShift, got %d", spy.calls)
		}
		if _, ok := spy.watched.(*configv1.APIServer); !ok {
			t.Fatalf("expected watch on *configv1.APIServer, got %T", spy.watched)
		}
	})
}
