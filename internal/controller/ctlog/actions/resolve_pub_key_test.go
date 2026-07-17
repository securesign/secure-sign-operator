package actions

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const testCTlogPublicKey = "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZFt6NEqMxaeU76lnlYzFUNjFQGHq\nNF46BPCTlP/FgfMZjN608cDXf3LM5hTbvNyCEabE+4MbOcEMXhDQUlYFvA==\n-----END PUBLIC KEY-----"

func TestCTlogResolvePubKey_CanHandle(t *testing.T) {
	t.Parallel()
	a := NewResolvePubKeyAction()
	t.Run("not ready", func(t *testing.T) {
		t.Parallel()
		instance := &rhtasv1.CTlog{}
		if a.CanHandle(t.Context(), instance) {
			t.Error("expected false when no condition set")
		}
	})
	t.Run("initialize phase", func(t *testing.T) {
		t.Parallel()
		instance := &rhtasv1.CTlog{}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String(),
		})
		if !a.CanHandle(t.Context(), instance) {
			t.Error("expected true in Initialize phase")
		}
	})
}

func TestCTlogResolvePubKey_Handle(t *testing.T) {
	t.Parallel()
	type want struct {
		result    *action.Result
		publicKey string
	}
	tests := []struct {
		name         string
		publicKeyRef *rhtasv1.SecretKeySelector
		publicKey    string
		secretData   map[string][]byte
		want         want
	}{
		{
			name: "resolve from signer secret",
			publicKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys"},
				Key:                  "public",
			},
			secretData: map[string][]byte{"public": []byte(testCTlogPublicKey)},
			want: want{
				result:    testAction.Continue(),
				publicKey: testCTlogPublicKey,
			},
		},
		{
			name: "unchanged — no status update",
			publicKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys"},
				Key:                  "public",
			},
			publicKey:  testCTlogPublicKey,
			secretData: map[string][]byte{"public": []byte(testCTlogPublicKey)},
			want: want{
				result:    testAction.Continue(),
				publicKey: testCTlogPublicKey,
			},
		},
		{
			name: "PublicKeyRef not set — requeue",
			want: want{
				result:    &action.Result{Result: reconcile.Result{RequeueAfter: 5 * time.Second}},
				publicKey: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			ctx := t.Context()

			instance := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Status: rhtasv1.CTlogStatus{
					PublicKeyRef: tt.publicKeyRef,
					PublicKey:    tt.publicKey,
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			}

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)

			if tt.secretData != nil {
				builder = builder.WithObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: tt.publicKeyRef.Name, Namespace: "default"},
					Data:       tt.secretData,
				})
			}
			c := builder.Build()

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())
			got := a.Handle(ctx, instance)

			g.Expect(got).To(Equal(tt.want.result))
			g.Expect(instance.Status.PublicKey).To(Equal(tt.want.publicKey))
		})
	}
}

func TestCTlogResolvePubKey_Handle_SecretReadError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
		Status: rhtasv1.CTlogStatus{
			PublicKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "nonexistent"},
				Key:                  "public",
			},
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).Build()

	a := testAction.PrepareAction(c, NewResolvePubKeyAction())
	got := a.Handle(ctx, instance)

	g.Expect(got).ToNot(BeNil())
	g.Expect(got.Result.RequeueAfter).To(Equal(5 * time.Second))
	g.Expect(instance.Status.PublicKey).To(BeEmpty())
}
