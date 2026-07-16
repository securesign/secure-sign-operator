package actions

import (
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSortByStatus(t *testing.T) {
	t.Parallel()

	allHealthy := []metav1.Condition{
		{Type: TrillianCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
		{Type: FulcioCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
		{Type: RekorCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
		{Type: CTlogCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
		{Type: TufCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
		{Type: TSACondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
	}

	t.Run("all healthy — first in the base list wins the tie", func(t *testing.T) {
		t.Parallel()
		sorted := sortByStatus(allHealthy)
		if sorted[0] != TrillianCondition {
			t.Fatalf("expected %s first, got %s", TrillianCondition, sorted[0])
		}
	})

	t.Run("a component genuinely earlier in its lifecycle still wins by Reason", func(t *testing.T) {
		t.Parallel()
		conditions := append([]metav1.Condition{}, allHealthy...)
		meta.SetStatusCondition(&conditions, metav1.Condition{Type: RekorCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()})
		sorted := sortByStatus(conditions)
		if sorted[0] != RekorCondition {
			t.Fatalf("expected %s first, got %s", RekorCondition, sorted[0])
		}
	})

	t.Run("Reason=Ready but Status=False (trust material drift) still outranks genuinely healthy components", func(t *testing.T) {
		t.Parallel()
		conditions := append([]metav1.Condition{}, allHealthy...)
		// Simulate trustmaterial's drift signal: Ready reason preserved, Status flipped False.
		meta.SetStatusCondition(&conditions, metav1.Condition{Type: FulcioCondition, Status: metav1.ConditionFalse, Reason: state.Ready.String()})
		sorted := sortByStatus(conditions)
		if sorted[0] != FulcioCondition {
			t.Fatalf("expected drifted %s to be picked as the worst condition, got %s", FulcioCondition, sorted[0])
		}
	})

	t.Run("a False/NotReady condition always outranks a genuinely-earlier-lifecycle True one", func(t *testing.T) {
		t.Parallel()
		conditions := append([]metav1.Condition{}, allHealthy...)
		meta.SetStatusCondition(&conditions, metav1.Condition{Type: CTlogCondition, Status: metav1.ConditionFalse, Reason: state.Ready.String()})
		meta.SetStatusCondition(&conditions, metav1.Condition{Type: RekorCondition, Status: metav1.ConditionTrue, Reason: state.Initialize.String()})
		sorted := sortByStatus(conditions)
		if sorted[0] != CTlogCondition {
			t.Fatalf("expected %s (Status=False) first regardless of Reason ordering, got %s", CTlogCondition, sorted[0])
		}
	})
}

func TestUpdateStatusAction_Handle_PropagatesDrift(t *testing.T) {
	t.Parallel()

	instance := &rhtasv1.Securesign{
		ObjectMeta: metav1.ObjectMeta{Name: "test-securesign", Namespace: "default"},
		Status: rhtasv1.SecuresignStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: TrillianCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: FulcioCondition, Status: metav1.ConditionFalse, Reason: state.Ready.String(), Message: "trust material drifted"},
				{Type: RekorCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: CTlogCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: TufCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: TSACondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(c, NewUpdateStatusAction())

	if !a.CanHandle(t.Context(), instance) {
		t.Fatal("expected CanHandle to be true")
	}
	a.Handle(t.Context(), instance)

	ready := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	if ready == nil {
		t.Fatal("expected Ready condition to be set")
	}
	if ready.Status != metav1.ConditionFalse {
		t.Fatalf("expected umbrella Ready to be False when a child has drifted, got %s", ready.Status)
	}
	if ready.Reason != state.Ready.String() {
		t.Fatalf("expected Ready reason to be preserved as %q, got %q", state.Ready.String(), ready.Reason)
	}
}
