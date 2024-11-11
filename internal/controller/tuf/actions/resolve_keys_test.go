package actions

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	common "github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
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

	g.Expect(testAction.Client.Create(testContext, kubernetes.CreateSecret("testSecret", t.Name(),
		map[string][]byte{"key": nil}, map[string]string{labels.LabelNamespace + "/rekor.pub": "key"}))).To(Succeed())
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
	g.Expect(testAction.Client.Create(testContext, kubernetes.CreateSecret("new", t.Name(),
		map[string][]byte{"key": nil}, map[string]string{labels.LabelNamespace + "/ctfe.pub": "key"}))).To(Succeed())
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
