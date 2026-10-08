package server

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResolveKMSSigner_ClearsStaleStatus(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := rekorInstance()
	instance.Spec.Signer.Type = rhtasv1.SignerTypeKMS
	instance.Spec.Signer.Kms = &rhtasv1.KMS{KeyResource: "awskms://key"}
	instance.Status.Signer = rhtasv1.RekorSignerStatus{
		KeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "file-signer"},
			Key:                  constants.KeyPrivate,
		},
		PasswordRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-password"},
			Key:                  "password",
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer.KeyRef).To(BeNil())
	g.Expect(instance.Status.Signer.PasswordRef).To(BeNil())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).To(BeTrue())

	updated := &rhtasv1.Rekor{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), updated)).To(Succeed())
	g.Expect(updated.Status.Signer.KeyRef).To(BeNil())
	g.Expect(updated.Status.Signer.PasswordRef).To(BeNil())
}

func TestResolveKMSSigner_DisabledForFileType(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()
	instance.Spec.Signer.Type = rhtasv1.SignerTypeSecret

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())

	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestResolveKMSSigner_ConditionAlreadySatisfied(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()
	instance.Generation = 1
	instance.Spec.Signer.Type = rhtasv1.SignerTypeKMS
	instance.Spec.Signer.Kms = &rhtasv1.KMS{KeyResource: "awskms://key"}
	instance.Status.Conditions[1] = metav1.Condition{
		Type:               actions.SignerCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: instance.Generation,
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())

	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestResolveKMSSigner_RequiresPendingReadyState(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()
	instance.Spec.Signer.Type = rhtasv1.SignerTypeKMS
	instance.Spec.Signer.Kms = &rhtasv1.KMS{KeyResource: "awskms://key"}
	instance.Status.Conditions[0] = metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: state.Ready.String(),
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())

	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}
