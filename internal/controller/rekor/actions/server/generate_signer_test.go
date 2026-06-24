package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1 "github.com/securesign/operator/api/v1"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateSigner_CanHandle(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		status     []metav1.Condition
		canHandle  bool
	}{
		{
			name:       "no signer condition",
			generation: 1,
			canHandle:  true,
		},
		{
			name:       "condition false",
			generation: 1,
			status: []metav1.Condition{
				{Type: actions.SignerCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
			},
			canHandle: true,
		},
		{
			name:       "condition unknown",
			generation: 1,
			status: []metav1.Condition{
				{Type: actions.SignerCondition, Status: metav1.ConditionUnknown, Reason: state.Ready.String()},
			},
			canHandle: true,
		},
		{
			name:       "condition true with matching generation",
			generation: 1,
			status: []metav1.Condition{
				{Type: actions.SignerCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String(), ObservedGeneration: 1},
			},
			canHandle: false,
		},
		{
			name:       "condition true with stale generation",
			generation: 2,
			status: []metav1.Condition{
				{Type: actions.SignerCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String(), ObservedGeneration: 1},
			},
			canHandle: true,
		},
		{
			name:       "condition true with zero observed generation",
			generation: 1,
			status: []metav1.Condition{
				{Type: actions.SignerCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
			canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewGenerateSignerAction())
			instance := rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Generation: tt.generation},
			}
			for _, status := range tt.status {
				meta.SetStatusCondition(&instance.Status.Conditions, status)
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestGenerateSigner_Handle(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		spec    rhtasv1.RekorSigner
		status  rhtasv1.RekorSignerStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1.Rekor)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "use spec.signer.keyRef",
			env: env{
				spec: rhtasv1.RekorSigner{
					KeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "private"},
				},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.Signer.KeyRef.Key).Should(Equal("private"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "generate signer key - default KMS",
			env: env{
				spec:   rhtasv1.RekorSigner{},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(ContainSubstring("rekor-signer-rekor-"))

					g.Expect(instance.Status.Signer.PasswordRef).Should(BeNil())

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "generate signer key - KMS secret",
			env: env{
				spec:   rhtasv1.RekorSigner{KMS: "secret"},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(ContainSubstring("rekor-signer-rekor-"))

					g.Expect(instance.Status.Signer.PasswordRef).Should(BeNil())

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "replace status.signer.keyRef from spec",
			env: env{
				spec: rhtasv1.RekorSigner{
					KeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "new_secret"}, Key: "private"},
				},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(Equal("new_secret"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "use existing signer key",
			env: env{
				spec:   rhtasv1.RekorSigner{},
				status: rhtasv1.RekorSignerStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secret",
							Namespace: "default",
							Labels:    map[string]string{RekorSignerLabel: "private"},
						},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.Signer.KeyRef.Key).Should(Equal("private"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "use spec.signer.KMS",
			env: env{
				spec: rhtasv1.RekorSigner{
					KMS: "awskms://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1",
				},
				status: rhtasv1.RekorSignerStatus{
					KeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old_secret"}, Key: "private"},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).Should(BeNil())
					g.Expect(instance.Status.Signer.PasswordRef).Should(BeNil())

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "spec with encrypted private key",
			env: env{
				spec: rhtasv1.RekorSigner{
					KeyRef:      &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.Signer.KeyRef.Key).Should(Equal("private"))

					g.Expect(instance.Status.Signer.PasswordRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.PasswordRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.Signer.PasswordRef.Key).Should(Equal("password"))

					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).Should(BeTrue())
					g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, actions.ServerCondition)).Should(BeTrue())
				},
			},
		},
		{
			name: "unrelated spec change re-stamps secret signer generation",
			env: env{
				spec: rhtasv1.RekorSigner{
					KeyRef:      &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
				status: rhtasv1.RekorSignerStatus{
					KeyRef:      &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "private"},
					PasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "secret"}, Key: "password"},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).Should(Equal("secret"))
					g.Expect(instance.Status.Signer.PasswordRef).ShouldNot(BeNil())
					g.Expect(instance.Status.Signer.PasswordRef.Name).Should(Equal("secret"))

					c := meta.FindStatusCondition(instance.Status.Conditions, actions.SignerCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.Status).Should(Equal(metav1.ConditionTrue))
					g.Expect(c.ObservedGeneration).Should(Equal(instance.Generation))
				},
			},
		},
		{
			name: "unrelated spec change re-stamps KMS signer generation",
			env: env{
				spec: rhtasv1.RekorSigner{
					KMS: "awskms://1234abcd-12ab-34cd-56ef-1234567890ab?region=us-east-1",
				},
				status: rhtasv1.RekorSignerStatus{},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, instance *rhtasv1.Rekor) {
					g.Expect(instance.Status.Signer.KeyRef).Should(BeNil())
					g.Expect(instance.Status.Signer.PasswordRef).Should(BeNil())

					c := meta.FindStatusCondition(instance.Status.Conditions, actions.SignerCondition)
					g.Expect(c).ShouldNot(BeNil())
					g.Expect(c.Status).Should(Equal(metav1.ConditionTrue))
					g.Expect(c.ObservedGeneration).Should(Equal(instance.Generation))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Spec: rhtasv1.RekorSpec{
					Signer: tt.env.spec,
				},
				Status: rhtasv1.RekorStatus{
					Signer: tt.env.status,
				},
			}

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: state.Pending.String(),
			})

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   actions.SignerCondition,
				Status: metav1.ConditionFalse,
				Reason: state.Pending.String(),
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			a := testAction.PrepareAction(c, NewGenerateSignerAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}

func TestGenerateSigner_SECURESIGN_1455(t *testing.T) {
	g := NewWithT(t)
	rekorNN := types.NamespacedName{Name: "rekor", Namespace: "default"}
	type env struct {
		status  rhtasv1.RekorSignerStatus
		objects []client.Object
	}
	type want struct {
		result *action.Result
		verify func(Gomega, client.WithWatch, <-chan watch.Event)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "link unassigned signer secret by rekor.signer.pem label",
			env: env{
				status: rhtasv1.RekorSignerStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unassigned-secret",
							Namespace: "default",
							Labels: map[string]string{
								RekorSignerLabel: "private",
							},
						},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					rekor := &rhtasv1.Rekor{}
					g.Expect(cli.Get(context.TODO(), rekorNN, rekor)).Should(Succeed())
					g.Expect(rekor.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(rekor.Status.Signer.KeyRef.Name).Should(Equal("unassigned-secret"))

					g.Expect(events).To(BeEmpty())
					for event := range events {
						g.Expect(event.Type).ShouldNot(Equal(watch.Added))
					}
				},
			},
		},
		{
			name: "create new signer secret",
			env: env{
				status: rhtasv1.RekorSignerStatus{},
				objects: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "unassigned-secret",
							Namespace: "default",
							Labels:    map[string]string{},
						},
					},
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(g Gomega, cli client.WithWatch, events <-chan watch.Event) {
					rekor := &rhtasv1.Rekor{}
					g.Expect(cli.Get(context.TODO(), rekorNN, rekor)).Should(Succeed())
					g.Expect(rekor.Status.Signer.KeyRef).ShouldNot(BeNil())
					g.Expect(rekor.Status.Signer.KeyRef.Name).ShouldNot(Equal("unassigned-secret"))

					g.Expect(events).To(HaveLen(1))
					for event := range events {
						g.Expect(event.Type).Should(Equal(watch.Added))
						g.Expect(event.Object.(*v1.Secret).GenerateName).Should(Equal(fmt.Sprintf(secretNameFormat, "rekor")))
					}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Status: rhtasv1.RekorStatus{
					Signer: tt.env.status,
				},
			}

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: state.Pending.String(),
			})

			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   actions.SignerCondition,
				Status: metav1.ConditionFalse,
				Reason: state.Pending.String(),
			})

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithObjects(tt.env.objects...).
				Build()

			watchSecrets, err := c.Watch(ctx, &v1.SecretList{}, client.InNamespace(instance.Namespace))
			g.Expect(err).ShouldNot(HaveOccurred())

			a := testAction.PrepareAction(c, NewGenerateSignerAction())

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}

			// second execution should not modify result
			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("second Handle() = %v, want %v", got, tt.want.result)
			}

			watchSecrets.Stop()
			if tt.want.verify != nil {
				tt.want.verify(g, c, watchSecrets.ResultChan())
			}
		})
	}
}
