package actions

import (
	"context"
	"crypto/elliptic"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	common "github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testAction = resolveKeysAction{
	BaseAction: common.BaseAction{
		Client:   fake.NewFakeClient(),
		Recorder: record.NewFakeRecorder(3),
		Logger:   logr.Logger{},
	},
}

var testContext = context.TODO()

func TestKeyAutogenerate(t *testing.T) {
	g := NewWithT(t)

	g.Expect(testAction.Client.Create(testContext, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testSecret",
			Namespace: t.Name(),
			Labels:    map[string]string{labels.LabelNamespace + "/rekor.pub": "key"},
		},
		Data: map[string][]byte{"key": nil},
	})).To(Succeed())
	instance := &v1alpha1.Tuf{Spec: v1alpha1.TufSpec{Keys: []v1alpha1.TufKey{
		{
			Name: "rekor.pub",
		},
	}},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.Ready,
				Reason: constants.Pending,
				Status: metav1.ConditionFalse,
			},
		}}}
	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("testSecret"))
	g.Expect(instance.Status.Keys[0].SecretRef.Key).To(Equal("key"))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyProvided(t *testing.T) {
	g := NewWithT(t)
	instance := &v1alpha1.Tuf{Spec: v1alpha1.TufSpec{Keys: []v1alpha1.TufKey{
		{
			Name: "rekor.pub",
			SecretRef: &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: "secret",
				},
				Key: "key",
			},
		},
	}},
		Status: v1alpha1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.Ready,
				Reason: constants.Pending,
				Status: metav1.ConditionFalse,
			}}}}
	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0]).To(Equal(instance.Spec.Keys[0]))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyUpdate(t *testing.T) {
	g := NewWithT(t)
	instance := &v1alpha1.Tuf{
		Spec: v1alpha1.TufSpec{Keys: []v1alpha1.TufKey{
			{
				Name: "rekor.pub",
				SecretRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "new",
					},
					Key: "key",
				},
			},
		}},
		Status: v1alpha1.TufStatus{Keys: []v1alpha1.TufKey{
			{
				Name: "rekor.pub",
				SecretRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "old",
					},
					Key: "key",
				},
			},
		},
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Pending,
					Status: metav1.ConditionFalse,
				}}}}

	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))
	g.Expect(instance.Status.Keys[0]).To(Equal(instance.Spec.Keys[0]))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyDelete(t *testing.T) {
	g := NewWithT(t)
	g.Expect(testAction.Client.Create(testContext, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new",
			Namespace: t.Name(),
			Labels:    map[string]string{labels.LabelNamespace + "/ctfe.pub": "key"},
		},
		Data: map[string][]byte{"key": nil},
	})).To(Succeed())
	instance := &v1alpha1.Tuf{
		Spec: v1alpha1.TufSpec{Keys: []v1alpha1.TufKey{
			{
				Name:      "ctfe.pub",
				SecretRef: nil,
			},
		}},
		Status: v1alpha1.TufStatus{Keys: []v1alpha1.TufKey{
			{
				Name: "ctfe.pub",
				SecretRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "old",
					},
					Key: "key",
				},
			},
		},
			Conditions: []metav1.Condition{
				{
					Type:   constants.Ready,
					Reason: constants.Pending,
					Status: metav1.ConditionFalse,
				},
			}}}

	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef).To(Not(BeNil()))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "ctfe.pub")).To(BeTrue())
}

func TestKeyValidationFailsInFIPS(t *testing.T) {
	g := NewWithT(t)
	cryptoutil.FIPSEnabled = true
	t.Cleanup(func() {
		cryptoutil.FIPSEnabled = false
	})

	invalidPub, _, _, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(testAction.Client.Create(testContext, &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: t.Name(),
		},
		Data: map[string][]byte{"key": invalidPub},
	})).To(Succeed())

	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tuf",
			Namespace: t.Name(),
		},
		Spec: v1alpha1.TufSpec{
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
					SecretRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "invalid",
						},
						Key: "key",
					},
				},
			},
		},
		Status: v1alpha1.TufStatus{
			Conditions: []metav1.Condition{{
				Type:   constants.Ready,
				Reason: constants.Pending,
				Status: metav1.ConditionFalse,
			}},
		},
	}

	testAction.Handle(testContext, instance)

	g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
	g.Expect(meta.FindStatusCondition(instance.Status.Conditions, "rekor.pub")).To(
		gstruct.PointTo(SatisfyAll(
			HaveField("Reason", Equal(constants.Failure)),
			HaveField("Message", ContainSubstring("FIPS")),
		)),
	)
}
