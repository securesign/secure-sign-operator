package server

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/fips"
	v2 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace  = "default"
	testPrivateKey = "private"
)

func createRekorInstance() *rhtasv1.Rekor {
	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rekor",
			Namespace: testNamespace,
		},
		Spec: rhtasv1.RekorSpec{
			Trillian: rhtasv1.TrillianService{
				Port: ptr.To[int32](8091),
			},
			Signer: rhtasv1.RekorSigner{
				KMS: signerKMSSecret,
			},
			SearchIndex: rhtasv1.SearchIndex{
				Create: ptr.To(true),
			},
		},
		Status: rhtasv1.RekorStatus{
			TreeID:          ptr.To[int64](123456),
			ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "test-config"},
			Signer: rhtasv1.RekorSignerStatus{
				KeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-secret"},
					Key:                  testPrivateKey,
				},
			},
		},
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.ServerCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	return instance
}

func TestFIPSClientSigningAlgorithms(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	instance := createRekorInstance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	dep := &v2.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      actions.ServerDeploymentName,
		Namespace: testNamespace,
	}, dep)).To(Succeed())

	container := dep.Spec.Template.Spec.Containers[0]
	g.Expect(container.Args).To(ContainElement("--client-signing-algorithms"))
	g.Expect(container.Args).To(ContainElement(fips.ClientSigningAlgorithms))
}

func TestNonFIPSNoClientSigningAlgorithms(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	original := fips.Enabled
	fips.Enabled = func() bool { return false }
	t.Cleanup(func() { fips.Enabled = original })

	instance := createRekorInstance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	dep := &v2.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      actions.ServerDeploymentName,
		Namespace: testNamespace,
	}, dep)).To(Succeed())

	container := dep.Spec.Template.Spec.Containers[0]
	g.Expect(container.Args).ToNot(ContainElement("--client-signing-algorithms"))
}

func TestDeployAction_Handle_PreservesCachedPublicKeyOnDeploymentChange(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	instance := createRekorInstance()
	instance.Status.PublicKey = "-----BEGIN PUBLIC KEY-----\nOLDKEY\n-----END PUBLIC KEY-----\n"

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	g.Expect(instance.Status.PublicKey).To(Equal("-----BEGIN PUBLIC KEY-----\nOLDKEY\n-----END PUBLIC KEY-----\n"),
		"creating/updating the Deployment must not clobber the cached public key — trustmaterial owns that field and needs the prior value to detect drift")
}

func TestDeployAction_Handle_DefaultTrillianAddress(t *testing.T) {
	ctx := t.Context()
	g := NewWithT(t)

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rekor",
			Namespace: testNamespace,
		},
		Spec: rhtasv1.RekorSpec{
			Trillian: rhtasv1.TrillianService{
				Port: ptr.To[int32](8091),
			},
			Signer: rhtasv1.RekorSigner{
				KMS: signerKMSSecret,
			},
			SearchIndex: rhtasv1.SearchIndex{
				Create: ptr.To(true),
			},
		},
		Status: rhtasv1.RekorStatus{
			TreeID:          ptr.To[int64](123456),
			ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "test-config"},
			Signer: rhtasv1.RekorSignerStatus{
				KeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-secret"},
					Key:                  testPrivateKey,
				},
			},
		},
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.ServerCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewDeployAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	dep := &v2.Deployment{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      actions.ServerDeploymentName,
		Namespace: testNamespace,
	}, dep)).To(Succeed())

	container := dep.Spec.Template.Spec.Containers[0]
	g.Expect(container.Args).To(ContainElement("dns:///" + actions2.LogserverDeploymentName + "." + testNamespace + ".svc"))
	g.Expect(container.Args).To(ContainElement(`{"loadBalancingConfig":[{"round_robin":{}}]}`))
}
