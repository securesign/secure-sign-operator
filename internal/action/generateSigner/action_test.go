package generateSigner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

var (
	testData = map[string][]byte{
		"private": []byte("test-private-key"),
		"public":  []byte("test-public-key"),
	}
	precomputedHash = ComputeDataHash(testData)
)

func testWrapper(hasUser, isEnabled bool, generateErr error) func(*rhtasv1.Rekor) *wrapper[*rhtasv1.Rekor] {
	return Wrapper(Config[*rhtasv1.Rekor]{
		Resolve: func(_ context.Context, r *rhtasv1.Rekor, _ client.Client) bool {
			if !hasUser {
				return false
			}
			r.Status.Signer = rhtasv1.RekorSignerStatus{
				KeyRef: r.Spec.Signer.KeyRef.DeepCopy(),
			}
			return true
		},
		GenerateData: func(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) (map[string][]byte, error) {
			if generateErr != nil {
				return nil, generateErr
			}
			return testData, nil
		},
		AlignStatus: func(r *rhtasv1.Rekor, secret *corev1.Secret) {
			r.Status.Signer.KeyRef = &rhtasv1.SecretKeySelector{
				Key:                  "private",
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: secret.Name},
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

func matchingSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName(), Namespace: testNamespace,
			Annotations: map[string]string{
				annotations.DataHash: precomputedHash,
			},
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

func newAction(hasUser, enabled bool, genErr error) *testActionSetup {
	return &testActionSetup{hasUser: hasUser, enabled: enabled, genErr: genErr}
}

type testActionSetup struct {
	hasUser, enabled bool
	genErr           error
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
				testWrapper(false, tt.isEnabled, nil),
			))
			g.Expect(a.CanHandle(context.TODO(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestHandle(t *testing.T) {
	type env struct {
		instance  func() *rhtasv1.Rekor
		objects   []client.Object
		action    *testActionSetup
		intercept interceptor.Funcs
	}
	type want struct {
		isTerminal bool
		isError    bool
		errSubstr  string
		requeues   bool
		verify     func(Gomega, *rhtasv1.Rekor, client.Client)
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
				action:   newAction(false, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, cli client.Client) {
					g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())

					secret := &corev1.Secret{}
					g.Expect(cli.Get(context.TODO(), client.ObjectKeyFromObject(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName(), Namespace: testNamespace},
					}), secret)).To(Succeed())
					g.Expect(secret.Immutable).ToNot(BeNil())
					g.Expect(*secret.Immutable).To(BeTrue())
					g.Expect(secret.Data).To(HaveKey("private"))
					g.Expect(secret.Data).To(HaveKey("public"))
					g.Expect(secret.Annotations).To(HaveKey(annotations.DataHash))
					g.Expect(secret.Annotations[annotations.DataHash]).To(Equal(precomputedHash))
				},
			},
		},
		{
			name: "user-provided secret syncs status without creating secret",
			env: env{
				instance: func() *rhtasv1.Rekor {
					i := testInstance(pendingConditions()...)
					i.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"}, Key: "private",
					}
					return i
				},
				action: newAction(true, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, cli client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("user-secret"))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())

					secret := &corev1.Secret{}
					err := cli.Get(context.TODO(), client.ObjectKeyFromObject(&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: secretName(), Namespace: testNamespace},
					}), secret)
					g.Expect(err).To(HaveOccurred())
				},
			},
		},
		{
			name: "existing secret with matching hash is reused",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:  []client.Object{matchingSecret()},
				action:   newAction(false, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, testCondition)).To(BeTrue())
				},
			},
		},
		{
			name: "tampered data produces TerminalError",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects: []client.Object{func() *corev1.Secret {
					s := matchingSecret()
					s.Data = map[string][]byte{"private": []byte("tampered")}
					return s
				}()},
				action: newAction(false, true, nil),
			},
			want: want{isTerminal: true, errSubstr: "modified externally"},
		},
		{
			name: "status lost but secret exists by deterministic name is reused",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:  []client.Object{matchingSecret()},
				action:   newAction(false, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
				},
			},
		},
		{
			name: "generation error requeues",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   newAction(false, true, fmt.Errorf("key generation failed")),
			},
			want: want{requeues: true},
		},
		{
			name: "missing hash annotation skips hash check",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects: []client.Object{func() *corev1.Secret {
					s := matchingSecret()
					delete(s.Annotations, annotations.DataHash)
					return s
				}()},
				action: newAction(false, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
				},
			},
		},
		{
			name: "Get transient error propagates",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				action:   newAction(false, true, nil),
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
				action:   newAction(false, true, nil),
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
			name: "Create AlreadyExists with matching hash reuses secret",
			env: env{
				instance: func() *rhtasv1.Rekor { return testInstance(pendingConditions()...) },
				objects:  []client.Object{matchingSecret()},
				action:   newAction(false, true, nil),
				intercept: cacheLagInterceptor(),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
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
				action: newAction(false, true, nil),
			},
			want: want{
				verify: func(g Gomega, instance *rhtasv1.Rekor, _ client.Client) {
					g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal(secretName()))
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
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
				testWrapper(tt.env.action.hasUser, tt.env.action.enabled, tt.env.action.genErr),
			))

			result := a.Handle(ctx, instance)

			switch {
			case tt.want.isTerminal:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).ToNot(BeNil())
				g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
				if tt.want.errSubstr != "" {
					g.Expect(result.Err.Error()).To(ContainSubstring(tt.want.errSubstr))
				}
			case tt.want.isError:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).ToNot(BeNil())
				if tt.want.errSubstr != "" {
					g.Expect(result.Err.Error()).To(ContainSubstring(tt.want.errSubstr))
				}
			case tt.want.requeues:
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Result.RequeueAfter).To(BeNumerically(">", 0))
				g.Expect(result.Err).To(BeNil())
			default:
				g.Expect(result).To(Equal(testAction.Return()))
			}

			if tt.want.verify != nil {
				tt.want.verify(g, instance, cli)
			}
		})
	}
}

func TestComputeDataHash_Deterministic(t *testing.T) {
	g := NewWithT(t)

	h1 := ComputeDataHash(map[string][]byte{"b": []byte("val-b"), "a": []byte("val-a")})
	h2 := ComputeDataHash(map[string][]byte{"b": []byte("val-b"), "a": []byte("val-a")})
	g.Expect(h1).To(Equal(h2))
	g.Expect(h1).ToNot(BeEmpty())
	g.Expect(ComputeDataHash(map[string][]byte{"a": []byte("val-a"), "b": []byte("CHANGED")})).ToNot(Equal(h1))
}
