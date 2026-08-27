package tlsadherence

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testComponent    = "test-component"
	testInstanceName = "test-instance"
	testNamespace    = "default"
)

func testInstance(adherence string) *rhtasv1.Fulcio {
	instance := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testInstanceName,
			Namespace:  testNamespace,
			Generation: 1,
		},
	}
	if adherence != "" {
		instance.Annotations = map[string]string{annotations.TLSAdherence: adherence}
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String(),
	})
	return instance
}

func compliant(_ *rhtasv1.Fulcio) bool    { return true }
func nonCompliant(_ *rhtasv1.Fulcio) bool { return false }

// withClusterTLSProfile sets whether the operator resolves the cluster TLS
// security profile for the duration of a test, restoring the globals afterwards.
// The adherence gate only fires when resolution is active.
func withClusterTLSProfile(t *testing.T, active bool) {
	t.Helper()
	prevOpenshift, prevDisabled := config.Openshift, config.DisableClusterTLSProfile
	config.Openshift, config.DisableClusterTLSProfile = active, !active
	t.Cleanup(func() {
		config.Openshift, config.DisableClusterTLSProfile = prevOpenshift, prevDisabled
	})
}

// withClusterAdherenceStrict sets the cluster-wide TLS adherence floor for the
// duration of a test, restoring it afterwards.
func withClusterAdherenceStrict(t *testing.T, strict bool) {
	t.Helper()
	prev := config.ClusterTLSAdherenceStrict
	config.ClusterTLSAdherenceStrict = strict
	t.Cleanup(func() { config.ClusterTLSAdherenceStrict = prev })
}

func TestIntentFromAnnotations(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want annotationIntent
	}{
		{"nil is absent", nil, annotationAbsent},
		{"empty value is absent", map[string]string{annotations.TLSAdherence: ""}, annotationAbsent},
		{"unrecognised value is absent", map[string]string{annotations.TLSAdherence: "bogus"}, annotationAbsent},
		{"legacy is legacy", map[string]string{annotations.TLSAdherence: "legacy"}, annotationLegacy},
		{"strict is strict", map[string]string{annotations.TLSAdherence: "strict"}, annotationStrict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(intentFromAnnotations(tt.in)).To(Equal(tt.want))
		})
	}
}

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name          string
		clusterStrict bool
		intent        annotationIntent
		wantMode      Mode
		wantConflict  bool
	}{
		{"cluster strict + absent inherits strict", true, annotationAbsent, ModeStrict, false},
		{"cluster strict + strict affirms strict", true, annotationStrict, ModeStrict, false},
		{"cluster strict + legacy is a conflict", true, annotationLegacy, ModeStrict, true},
		{"cluster not strict + absent is legacy", false, annotationAbsent, ModeLegacy, false},
		{"cluster not strict + legacy is legacy", false, annotationLegacy, ModeLegacy, false},
		{"cluster not strict + strict tightens", false, annotationStrict, ModeStrict, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mode, conflict := resolveMode(tt.clusterStrict, tt.intent)
			g.Expect(mode).To(Equal(tt.wantMode))
			g.Expect(conflict).To(Equal(tt.wantConflict))
		})
	}
}

func TestCanHonourClusterTLSProfile_StubIsCompliant(t *testing.T) {
	g := NewWithT(t)
	// TODO: stub currently reports every component as compliant.
	g.Expect(CanHonourClusterTLSProfile(testInstance(""))).To(BeTrue())
}

func TestCanHandle(t *testing.T) {
	t.Run("enforced: compliant component is a no-op (CanHandle false)", func(t *testing.T) {
		g := NewWithT(t)
		withClusterTLSProfile(t, true)
		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, compliant))
		g.Expect(a.CanHandle(t.Context(), testInstance(""))).To(BeFalse())
	})

	t.Run("enforced: non-compliant component is handled (CanHandle true)", func(t *testing.T) {
		g := NewWithT(t)
		withClusterTLSProfile(t, true)
		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))
		g.Expect(a.CanHandle(t.Context(), testInstance(""))).To(BeTrue())
	})

	t.Run("disabled: non-compliant is a no-op regardless of annotation (CanHandle false)", func(t *testing.T) {
		g := NewWithT(t)
		withClusterTLSProfile(t, false)
		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))
		g.Expect(a.CanHandle(t.Context(), testInstance("strict"))).To(BeFalse())
		g.Expect(a.CanHandle(t.Context(), testInstance("legacy"))).To(BeFalse())
		g.Expect(a.CanHandle(t.Context(), testInstance(""))).To(BeFalse())
	})

	t.Run("stale condition is handled even when disabled (CanHandle true)", func(t *testing.T) {
		g := NewWithT(t)
		withClusterTLSProfile(t, false)
		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))
		g.Expect(a.CanHandle(t.Context(), instanceWithAdherenceCondition())).To(BeTrue())
	})

	t.Run("stale condition is handled even when compliant (CanHandle true)", func(t *testing.T) {
		g := NewWithT(t)
		withClusterTLSProfile(t, true)
		cli := testAction.FakeClientBuilder().Build()
		a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, compliant))
		g.Expect(a.CanHandle(t.Context(), instanceWithAdherenceCondition())).To(BeTrue())
	})
}

func TestHandle_LegacyDeploysWithWarning(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, true)
	withClusterAdherenceStrict(t, false)
	instance := testInstance("legacy")

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))

	result := a.Handle(t.Context(), instance)

	// Legacy mode must not block the rollout: no error is returned.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	warn := meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)
	g.Expect(warn).ToNot(BeNil())
	g.Expect(warn.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(warn.Reason).To(Equal(ReasonWarning))

	// Ready must not be flipped to Failure in legacy mode.
	ready := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	g.Expect(ready.Reason).ToNot(Equal(state.Failure.String()))
}

func TestHandle_StrictBlocksRollout(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, true)
	withClusterAdherenceStrict(t, false)
	instance := testInstance("strict")

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))

	result := a.Handle(t.Context(), instance)

	// Strict mode must block the rollout with a terminal error.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())

	blocked := meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)
	g.Expect(blocked).ToNot(BeNil())
	g.Expect(blocked.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(blocked.Reason).To(Equal(state.Failure.String()))

	// A terminal error also drives the component Ready condition to Failure.
	ready := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(ready.Reason).To(Equal(state.Failure.String()))
}

func TestHandle_ClusterStrictInheritedWithoutAnnotation(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, true)
	withClusterAdherenceStrict(t, true)
	instance := testInstance("") // no annotation: inherit the cluster mandate

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))

	result := a.Handle(t.Context(), instance)

	// Inheriting a strict cluster blocks, but it is not a configuration conflict.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())

	blocked := meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)
	g.Expect(blocked).ToNot(BeNil())
	g.Expect(blocked.Reason).To(Equal(state.Failure.String()))
}

func TestHandle_ClusterStrictWithLegacyAnnotationIsConflict(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, true)
	withClusterAdherenceStrict(t, true)
	instance := testInstance("legacy") // tries to relax below the cluster floor

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))

	result := a.Handle(t.Context(), instance)

	// Relaxing below the cluster floor is a terminal configuration error.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())

	cond := meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(ReasonInvalidConfiguration))
}

// instanceWithAdherenceCondition returns an instance carrying a stale
// TLSProfileAdherence warning condition, as if left over from a previous
// reconcile when the component was gated.
func instanceWithAdherenceCondition() *rhtasv1.Fulcio {
	instance := testInstance("")
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    TLSProfileAdherenceCondition,
		Status:  metav1.ConditionFalse,
		Reason:  ReasonWarning,
		Message: "stale condition from a previous reconcile",
	})
	return instance
}

func TestHandle_CleansUpStaleConditionWhenDisabled(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, false) // flag switched back to disabled
	instance := instanceWithAdherenceCondition()

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, nonCompliant))

	result := a.Handle(t.Context(), instance)

	// Cleanup must not error and must remove the stale condition.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)).To(BeNil())
}

func TestHandle_CleansUpStaleConditionWhenNowCompliant(t *testing.T) {
	g := NewWithT(t)
	withClusterTLSProfile(t, true)
	instance := instanceWithAdherenceCondition()

	cli := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(cli, NewAction[*rhtasv1.Fulcio](testComponent, compliant))

	result := a.Handle(t.Context(), instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TLSProfileAdherenceCondition)).To(BeNil())
}
