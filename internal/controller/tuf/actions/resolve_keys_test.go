package actions

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	common "github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testAction = resolveKeysAction{
	BaseAction: common.BaseAction{
		Client:   fake.NewFakeClient(),
		Recorder: events.NewFakeRecorder(3),
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
	instance := &rhtasv1.Tuf{Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
		{
			Name: "rekor.pub",
		},
	}},
		Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Pending.String(),
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
	instance := &rhtasv1.Tuf{Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
		{
			Name: "rekor.pub",
			SecretRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: "secret",
				},
				Key: "key",
			},
		},
	}},
		Status: rhtasv1.TufStatus{Conditions: []metav1.Condition{
			{
				Type:   constants.ReadyCondition,
				Reason: state.Pending.String(),
				Status: metav1.ConditionFalse,
			}}}}
	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0]).To(Equal(instance.Spec.Keys[0]))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "rekor.pub")).To(BeTrue())
}

func TestKeyUpdate(t *testing.T) {
	g := NewWithT(t)
	instance := &rhtasv1.Tuf{
		Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
			{
				Name: "rekor.pub",
				SecretRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "new",
					},
					Key: "key",
				},
			},
		}},
		Status: rhtasv1.TufStatus{Keys: []rhtasv1.TufKey{
			{
				Name: "rekor.pub",
				SecretRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "old",
					},
					Key: "key",
				},
			},
		},
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Reason: state.Pending.String(),
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
	instance := &rhtasv1.Tuf{
		Spec: rhtasv1.TufSpec{Keys: []rhtasv1.TufKey{
			{
				Name:      "ctfe.pub",
				SecretRef: nil,
			},
		}},
		Status: rhtasv1.TufStatus{Keys: []rhtasv1.TufKey{
			{
				Name: "ctfe.pub",
				SecretRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "old",
					},
					Key: "key",
				},
			},
		},
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Reason: state.Pending.String(),
					Status: metav1.ConditionFalse,
				},
			}}}

	testAction.Handle(testContext, instance)

	g.Expect(instance.Status.Keys).To(HaveLen(1))
	g.Expect(instance.Status.Keys[0].SecretRef).To(Not(BeNil()))
	g.Expect(instance.Status.Keys[0].SecretRef.Name).To(Equal("new"))

	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, "ctfe.pub")).To(BeTrue())
}
