package controller

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestServiceRefWatch(t *testing.T) {
	ctlog := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "watched-ctlog",
			Namespace: "ns1",
		},
	}

	tests := []struct {
		name     string
		fulcios  []client.Object
		expected []reconcile.Request
	}{
		{
			name: "ref match enqueues request",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{Name: "watched-ctlog", Namespace: "ns1"},
						},
					},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: "f1", Namespace: "ns1"}},
			},
		},
		{
			name: "ref name mismatch returns nothing",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{Name: "other-ctlog", Namespace: "ns1"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "ref namespace mismatch returns nothing",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{Name: "watched-ctlog", Namespace: "other-ns"},
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "URL only does not enqueue",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							URL: "http://external-ctlog:8080",
						},
					},
				},
			},
			expected: nil,
		},
		{
			name: "empty ServiceReference autodiscovery same namespace",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{},
					},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: "f1", Namespace: "ns1"}},
			},
		},
		{
			name: "empty ServiceReference autodiscovery different namespace",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "other-ns"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{},
					},
				},
			},
			expected: nil,
		},
		{
			name: "multiple fulcios mixed refs",
			fulcios: []client.Object{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "match", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{Name: "watched-ctlog", Namespace: "ns1"},
						},
					},
				},
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "no-match", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{Name: "other-ctlog", Namespace: "ns1"},
						},
					},
				},
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{Name: "autodiscovery", Namespace: "ns1"},
					Spec: rhtasv1.FulcioSpec{
						Ctlog: rhtasv1.ServiceReference{},
					},
				},
			},
			expected: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Name: "match", Namespace: "ns1"}},
				{NamespacedName: types.NamespacedName{Name: "autodiscovery", Namespace: "ns1"}},
			},
		},
		{
			name:     "no fulcios returns nothing",
			fulcios:  nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			c := testAction.FakeClientBuilder().
				WithObjects(tt.fulcios...).
				Build()

			mapFn := ServiceRefWatch(c, &rhtasv1.FulcioList{}, func(o client.Object) rhtasv1.ServiceReference {
				return o.(*rhtasv1.Fulcio).Spec.Ctlog
			})

			got := mapFn(ctx, ctlog)

			if len(tt.expected) == 0 && len(got) == 0 {
				return
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d requests, got %d: %v", len(tt.expected), len(got), got)
			}
			gotSet := make(map[types.NamespacedName]bool, len(got))
			for _, r := range got {
				gotSet[r.NamespacedName] = true
			}
			for _, want := range tt.expected {
				if !gotSet[want.NamespacedName] {
					t.Errorf("expected request %v not found in %v", want.NamespacedName, got)
				}
			}
		})
	}
}
