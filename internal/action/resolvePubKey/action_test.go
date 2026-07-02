package resolvePubKey

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const validPEM = "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEtest\n-----END PUBLIC KEY-----\n"

type testResolver struct {
	resolve func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error)
}

func (r testResolver) ComponentName() string { return "rekor" }

func (r testResolver) ConditionType() string { return constants.ReadyCondition }

func (r testResolver) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r testResolver) GetTrustMaterial(instance *rhtasv1.Rekor) string {
	return instance.Status.PublicKey
}

func (r testResolver) SetTrustMaterial(instance *rhtasv1.Rekor, pem string) {
	instance.Status.PublicKey = pem
}

func (r testResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.Rekor) ([]byte, error) {
	if r.resolve != nil {
		return r.resolve(ctx, cli, instance)
	}
	return nil, nil
}

func newTestRekor(publicKey string) *rhtasv1.Rekor {
	return &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rekor", Namespace: "default"},
		Status: rhtasv1.RekorStatus{
			Url:       "http://rekor-server.default.svc",
			PublicKey: publicKey,
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
		},
	}
}

func TestResolvePubKey_CanHandle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		instance  *rhtasv1.Rekor
		canHandle bool
	}{
		{
			name: "server not ready — no condition",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
			},
			canHandle: false,
		},
		{
			name:      "server ready — initialize phase",
			instance:  newTestRekor(""),
			canHandle: true,
		},
		{
			name:      "already resolved — still handles for key rotation",
			instance:  newTestRekor("existing-key"),
			canHandle: true,
		},
		{
			name: "below initialize — pending phase",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
				Status: rhtasv1.RekorStatus{Conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				}},
			},
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewAction[*rhtasv1.Rekor](testResolver{}))
			g := NewWithT(t)
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestResolvePubKey_Handle(t *testing.T) {
	t.Parallel()

	type env struct {
		publicKey string
		resolve   func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error)
		intercept interceptor.Funcs
	}
	type want struct {
		result    *action.Result
		publicKey string
		hasError  bool
	}

	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "fresh resolve — sets status and continues",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				publicKey: validPEM,
			},
		},
		{
			name: "unchanged key — continues without status update",
			env: env{
				publicKey: validPEM,
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				result:    testAction.Continue(),
				publicKey: validPEM,
			},
		},
		{
			name: "key rotation — updates status",
			env: env{
				publicKey: "-----BEGIN PUBLIC KEY-----\nOLDKEYDATA1234AB\n-----END PUBLIC KEY-----\n",
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				publicKey: validPEM,
			},
		},
		{
			name: "resolve error — sets condition message and requeues",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("connection refused")
				},
			},
			want: want{
				result:    testAction.RequeueAfter(5),
				publicKey: "",
			},
		},
		{
			name: "invalid PEM from service — sets condition message and requeues",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte("not-pem-data"), nil
				},
			},
			want: want{
				result:    testAction.RequeueAfter(5),
				publicKey: "",
			},
		},
		{
			name: "PersistStatus failure — returns error for retry",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
				intercept: interceptor.Funcs{
					SubResourceUpdate: func(_ context.Context, _ client.Client, _ string, obj client.Object, _ ...client.SubResourceUpdateOption) error {
						return apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
					},
				},
			},
			want: want{
				publicKey: validPEM,
				hasError:  true,
			},
		},
		{
			name: "PersistStatus conflict — returns error for retry",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
				intercept: interceptor.Funcs{
					SubResourceUpdate: func(_ context.Context, _ client.Client, _ string, obj client.Object, _ ...client.SubResourceUpdateOption) error {
						return apierrors.NewConflict(schema.GroupResource{Resource: "rekors"}, obj.GetName(), fmt.Errorf("object modified"))
					},
				},
			},
			want: want{
				publicKey: validPEM,
				hasError:  true,
			},
		},
		{
			name: "transient resolve error preserves existing key",
			env: env{
				publicKey: validPEM,
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("temporary network issue")
				},
			},
			want: want{
				result:    testAction.RequeueAfter(5),
				publicKey: validPEM,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			instance := newTestRekor(tt.env.publicKey)

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			if tt.env.intercept.SubResourceUpdate != nil {
				builder = builder.WithInterceptorFuncs(tt.env.intercept)
			}
			c := builder.Build()

			a := testAction.PrepareAction(c, NewAction[*rhtasv1.Rekor](testResolver{resolve: tt.env.resolve}))
			result := a.Handle(t.Context(), instance)

			if tt.want.hasError {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).To(HaveOccurred())
				g.Expect(errors.Is(result.Err, ErrPersistStatus)).To(BeTrue())
			} else if tt.want.result != nil {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Result.RequeueAfter).ToNot(BeZero())
			}

			g.Expect(instance.Status.PublicKey).To(Equal(tt.want.publicKey))
		})
	}
}
