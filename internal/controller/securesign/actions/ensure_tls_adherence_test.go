package actions

import (
	"context"
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	testAction "github.com/securesign/operator/internal/testing/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// parentWithTLSAdherence returns a Securesign carrying the tlsAdherence
// annotation together with the minimal spec required for every ensure action to
// create its child (TimestampAuthority must be non-nil to be reconciled).
func parentWithTLSAdherence(value string) *rhtasv1.Securesign {
	return &rhtasv1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				annotations.TLSAdherence: value,
			},
		},
		Spec: rhtasv1.SecuresignSpec{
			TimestampAuthority: &rhtasv1.TimestampAuthoritySpec{},
		},
	}
}

func childAnnotations(t *testing.T, newAction func() action.Action[*rhtasv1.Securesign], child client.Object) map[string]string {
	t.Helper()
	parent := parentWithTLSAdherence("strict")
	c := testAction.FakeClientBuilder().WithObjects(parent).WithStatusSubresource(parent).Build()
	a := testAction.PrepareAction(c, newAction())

	a.Handle(context.Background(), parent)

	if err := c.Get(context.Background(), types.NamespacedName{Name: "test", Namespace: "default"}, child); err != nil {
		t.Fatalf("expected child resource to be created: %v", err)
	}
	return child.GetAnnotations()
}

// TestTLSAdherencePropagation asserts the tlsAdherence annotation is propagated
// from the Securesign umbrella to each of the components it manages.
func TestTLSAdherencePropagation(t *testing.T) {
	tests := []struct {
		name      string
		newAction func() action.Action[*rhtasv1.Securesign]
		child     client.Object
	}{
		{"fulcio", NewFulcioAction, &rhtasv1.Fulcio{}},
		{"rekor", NewRekorAction, &rhtasv1.Rekor{}},
		{"ctlog", NewCtlogAction, &rhtasv1.CTlog{}},
		{"trillian", NewTrillianAction, &rhtasv1.Trillian{}},
		{"tuf", NewTufAction, &rhtasv1.Tuf{}},
		{"tsa", NewTsaAction, &rhtasv1.TimestampAuthority{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := childAnnotations(t, tt.newAction, tt.child)
			if got[annotations.TLSAdherence] != "strict" {
				t.Fatalf("expected %s=strict on %s child, got %q",
					annotations.TLSAdherence, tt.name, got[annotations.TLSAdherence])
			}
		})
	}
}
