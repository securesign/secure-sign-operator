package generateSigner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testCondition    = "TestSignerCondition"
	testNameFormat   = "test-signer-config-%s"
	testComponent    = "test-component"
	testDeployment   = "test-deployment"
	testInstanceName = "test-instance"
	testNamespace    = "default"
)

var testData = map[string][]byte{
	"private": []byte("test-private-key"),
	"public":  []byte("test-public-key"),
}

func testWrapper(resolvedName string, isEnabled bool, generateErr error) func(*rhtasv1.Rekor) *wrapper[*rhtasv1.Rekor] {
	return testWrapperWithResolveErr(resolvedName, isEnabled, generateErr, nil)
}

func testWrapperWithResolveErr(resolvedName string, isEnabled bool, generateErr, resolveErr error) func(*rhtasv1.Rekor) *wrapper[*rhtasv1.Rekor] {
	return Wrapper(Config[*rhtasv1.Rekor]{
		ResolveRef: func(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) (*rhtasv1.SecretKeySelector, error) {
			if resolveErr != nil {
				return nil, resolveErr
			}
			if resolvedName == "" {
				return nil, nil
			}
			return &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: resolvedName}}, nil
		},
		GenerateData: func(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) (map[string][]byte, error) {
			if generateErr != nil {
				return nil, generateErr
			}
			return testData, nil
		},
		AlignStatus: func(r *rhtasv1.Rekor, ref rhtasv1.SecretKeySelector) {
			r.Status.Signer.KeyRef = &rhtasv1.SecretKeySelector{
				Key:                  "private",
				LocalObjectReference: ref.LocalObjectReference,
			}
		},
		IsEnabled: func(_ *rhtasv1.Rekor) bool { return isEnabled },
	})
}

func testInstance(conditions ...metav1.Condition) *rhtasv1.Rekor {
	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name: testInstanceName, Namespace: testNamespace, Generation: 1,
		},
	}
	for _, c := range conditions {
		meta.SetStatusCondition(&instance.Status.Conditions, c)
	}
	return instance
}

func pendingConditions() []metav1.Condition {
	return []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
		{Type: testCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
	}
}

func secretName() string {
	return fmt.Sprintf(testNameFormat, testInstanceName)
}

func existingSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName(), Namespace: testNamespace,
		},
		Immutable: ptr.To(true),
		Data:      testData,
	}
}

// cacheLagInterceptor simulates cache lag: the first Get for the deterministic
// secret returns NotFound, Create returns AlreadyExists, and subsequent Gets succeed.
func cacheLagInterceptor() interceptor.Funcs {
	var once sync.Once
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.Secret); ok && key.Name == secretName() {
				notFound := false
				once.Do(func() { notFound = true })
				if notFound {
					return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.Secret); ok {
				return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, obj.GetName())
			}
			return c.Create(ctx, obj, opts...)
		},
	}
}

type testActionSetup struct {
	resolvedName string
	genErr       error
	resolveErr   error
}

func TestCanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  *rhtasv1.Rekor
		isEnabled bool
		canHandle bool
	}{
		{
			name: "no ReadyCondition", instance: testInstance(),
			isEnabled: true, canHandle: false,
		},
		{
			name: "ReadyCondition below Pending",
			instance: testInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.NotDefined.String(),
			}),
			isEnabled: true, canHandle: false,
		},
		{
			name: "not enabled", instance: testInstance(pendingConditions()...),
			isEnabled: false, canHandle: false,
		},
		{
			name: "no component condition",
			instance: testInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String(),
			}),
			isEnabled: true, canHandle: true,
		},
		{
			name: "component condition false", instance: testInstance(pendingConditions()...),
			isEnabled: true, canHandle: true,
		},
		{
			name: "component condition true, matching generation",
			instance: testInstance(
				metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				metav1.Condition{Type: testCondition, Status: metav1.ConditionTrue, Reason: "Resolved", ObservedGeneration: 1},
			),
			isEnabled: true, canHandle: false,
		},
		{
			name: "component condition true, stale generation",
			instance: testInstance(
				metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				metav1.Condition{Type: testCondition, Status: metav1.ConditionTrue, Reason: "Resolved", ObservedGeneration: 0},
			),
			isEnabled: true, canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewAction(
				testCondition, testNameFormat, testComponent, testDeployment,
				testWrapper("", tt.isEnabled, nil),
			))
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestHandle(t *testing.T) {
	type env struct {
		instance  func() *rhtasv1.Rekor
		objects   []client.Object
		action    testActionSetup
		intercept interceptor.Funcs
	}
	type want struct {
		isTerminal bool
		isError    bool
		errSubstr  string
		wantErr    error
		requeues   bool
		verify     func(context.Context, Gomega, *rhtasv1.Rekor, client.Client)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "fresh install creates immutable secret",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   testActionSetup{},
			},
			want: want{
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Rekor, cli client.Client) {
					g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())

					secret := &corev1.Secret{}
					g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName(), Namespace: testNamespace},
					}), secret)).To(Succeed())
					g.Expect(secret.Immutable).ToNot(BeNil())
					g.Expect(*secret.Immutable).To(BeTrue())
					g.Expect(secret.Data).To(HaveKey("private"))
					g.Expect(secret.Data).To(HaveKey("public"))
				},
			},
		},
		{
			name: "resolved ref syncs status without creating secret",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   testActionSetup{resolvedName: "user-secret"},
			},
			want: want{
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Rekor, cli client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("user-secret"))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())

					secret := &corev1.Secret{}
					err := cli.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName(), Namespace: testNamespace},
					}), secret)
					g.Expect(err).To(HaveOccurred())
				},
			},
		},
		{
			name: "resolve error sets failure condition and pending ready",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action: testActionSetup{resolveErr: fmt.Errorf("%w: missing-secret",
					ErrSecretNotFound)},
			},
			want: want{
				isError: true,
				wantErr: ErrSecretNotFound,
				verify: func(_ context.Context, g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					cc := meta.FindStatusCondition(instance.Status.Conditions, testCondition)
					g.Expect(cc).ToNot(BeNil())
					g.Expect(cc.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(cc.Reason).To(Equal(state.Failure.String()))

					rc := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
					g.Expect(rc).ToNot(BeNil())
					g.Expect(rc.Reason).To(Equal(state.Pending.String()))
				},
			},
		},
		{
			name: "existing secret is reused",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:  []client.Object{existingSecret()},
				action:   testActionSetup{},
			},
			want: want{
				verify: func(_ context.Context, g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "status lost but secret exists by deterministic name is reused",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:  []client.Object{existingSecret()},
				action:   testActionSetup{},
			},
			want: want{
				verify: func(_ context.Context, g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
				},
			},
		},
		{
			name: "generation error requeues",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   testActionSetup{genErr: fmt.Errorf("key generation failed")},
			},
			want: want{requeues: true},
		},
		{
			name: "Get transient error propagates",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   testActionSetup{},
				intercept: interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*corev1.Secret); ok && key.Name == secretName() {
							return apierrors.NewInternalError(fmt.Errorf("api server unavailable"))
						}
						return c.Get(ctx, key, obj, opts...)
					},
				},
			},
			want: want{isError: true, errSubstr: "api server unavailable"},
		},
		{
			name: "Create failure propagates with condition",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   testActionSetup{},
				intercept: interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*corev1.Secret); ok && key.Name == secretName() {
							return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*corev1.Secret); ok {
							return apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, obj.GetName(), fmt.Errorf("quota exceeded"))
						}
						return c.Create(ctx, obj, opts...)
					},
				},
			},
			want: want{isError: true, errSubstr: "failed to create signer secret"},
		},
		{
			name: "Create AlreadyExists reuses secret",
			env: env{
				instance:  func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:   []client.Object{existingSecret()},
				action:    testActionSetup{},
				intercept: cacheLagInterceptor(),
			},
			want: want{
				verify: func(_ context.Context, g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "deleted status-referenced secret falls through to fresh install",
			env: env{
				instance: func() *rhtasv1.Rekor {
					i := testInstance(pendingConditions()...)
					i.Status.Signer.KeyRef = &rhtasv1.SecretKeySelector{
						Key:                  "private",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "rekor-signer-rekor-deleted"},
					}
					return i
				},
				action: testActionSetup{},
			},
			want: want{
				verify: func(_ context.Context, g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			instance := tt.env.instance()

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithInterceptorFuncs(tt.env.intercept)
			for _, obj := range tt.env.objects {
				builder = builder.WithObjects(obj)
			}
			cli := builder.Build()

			a := testAction.PrepareAction(cli, NewAction(
				testCondition, testNameFormat, testComponent, testDeployment,
				testWrapperWithResolveErr(tt.env.action.resolvedName, true, tt.env.action.genErr, tt.env.action.resolveErr),
			))

			result := a.Handle(ctx, instance)

			switch {
			case tt.want.isTerminal:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).To(HaveOccurred())
				g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
				if tt.want.wantErr != nil {
					g.Expect(errors.Is(result.Err, tt.want.wantErr)).To(BeTrue(),
						"expected error wrapping %v, got %v", tt.want.wantErr, result.Err)
				}
				if tt.want.errSubstr != "" {
					g.Expect(result.Err.Error()).To(ContainSubstring(tt.want.errSubstr))
				}
			case tt.want.isError:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).To(HaveOccurred())
				if tt.want.wantErr != nil {
					g.Expect(errors.Is(result.Err, tt.want.wantErr)).To(BeTrue(),
						"expected error wrapping %v, got %v", tt.want.wantErr, result.Err)
				}
				if tt.want.errSubstr != "" {
					g.Expect(result.Err.Error()).To(ContainSubstring(tt.want.errSubstr))
				}
			case tt.want.requeues:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Result.RequeueAfter).To(BeNumerically(">", 0))
				g.Expect(result.Err).ToNot(HaveOccurred())
			default:
				g.Expect(result).To(Equal(testAction.Return()))
			}

			if tt.want.verify != nil {
				tt.want.verify(ctx, g, instance, cli)
			}
		})
	}
}
