package actions

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	common "github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
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

func TestKeyAutogenerate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	g.Expect(testAction.Client.Create(ctx, &v1.Secret{
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
	testAction.Handle(ctx, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("testSecret"))
	g.Expect(instance.Status.Keys[0].SecretRef.Key).To(Equal("key"))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyProvided(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
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
	testAction.Handle(ctx, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0]).To(Equal(instance.Spec.Keys[0]))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyUpdate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
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

	testAction.Handle(ctx, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))
	g.Expect(instance.Status.Keys[0]).To(Equal(instance.Spec.Keys[0]))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyDelete(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	g.Expect(testAction.Client.Create(ctx, &v1.Secret{
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

	testAction.Handle(ctx, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef).To(Not(BeNil()))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "ctfe.pub")).To(BeTrue())
}
