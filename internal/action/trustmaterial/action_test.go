package trustmaterial

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

//go:embed testdata/public_key.pem
var validPEM string

type testResolver struct {
	resolve func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error)
	called  bool
}

func (r *testResolver) ComponentName() string { return "rekor" }

func (r *testResolver) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r *testResolver) GetTrustMaterial(instance *rhtasv1.Rekor) string {
	return instance.Status.PublicKey
}

func (r *testResolver) SetTrustMaterial(instance *rhtasv1.Rekor, pem string) {
	instance.Status.PublicKey = pem
}

func (r *testResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.Rekor) ([]byte, error) {
	r.called = true
	if r.resolve != nil {
		return r.resolve(ctx, cli, instance)
	}
	return nil, nil
}

func newTestRekor(publicKey string, conditions ...metav1.Condition) *rhtasv1.Rekor {
	if len(conditions) == 0 {
		conditions = []metav1.Condition{
			{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
		}
	}
	return &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rekor", Namespace: "default"},
		Status: rhtasv1.RekorStatus{
			Url:        "http://rekor-server.default.svc",
			PublicKey:  publicKey,
			Conditions: conditions,
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
			a := testAction.PrepareAction(c, NewAction[*rhtasv1.Rekor](&testResolver{}))
			g := NewWithT(t)
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestResolvePubKey_Handle(t *testing.T) {
	t.Parallel()

	type env struct {
		publicKey  string
		conditions []metav1.Condition
		resolve    func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error)
		intercept  interceptor.Funcs
	}
	type want struct {
		result        *action.Result
		publicKey     string
		hasError      bool
		resolveCalled bool
		condStatus    *metav1.ConditionStatus
	}

	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "first resolve — no condition, fetches and sets True",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionTrue),
			},
		},
		{
			name: "condition already True — still fetches and resolves every reconcile",
			env: env{
				publicKey: validPEM,
				conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					{Type: TrustMaterialCondition, Status: metav1.ConditionTrue, Reason: ReasonResolved,
						LastTransitionTime: metav1.Now()},
				},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionTrue),
			},
		},
		{
			name: "condition Unknown or False — still fetches and resolves",
			env: env{
				conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					{Type: TrustMaterialCondition, Status: metav1.ConditionFalse, Reason: ReasonResolveFailed,
						LastTransitionTime: metav1.Now()},
				},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionTrue),
			},
		},
		{
			name: "key rotation — detected immediately, does not update status without acknowledgement",
			env: env{
				publicKey: "-----BEGIN PUBLIC KEY-----\nOLDKEYDATA1234AB\n-----END PUBLIC KEY-----\n",
				conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					{Type: TrustMaterialCondition, Status: metav1.ConditionTrue, Reason: ReasonResolved,
						LastTransitionTime: metav1.Now()},
				},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte(validPEM), nil
				},
			},
			want: want{
				result:        testAction.Return(),
				publicKey:     "-----BEGIN PUBLIC KEY-----\nOLDKEYDATA1234AB\n-----END PUBLIC KEY-----\n",
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionFalse),
			},
		},
		{
			name: "resolve error — sets False and requeues",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("connection refused")
				},
			},
			want: want{
				result:        testAction.RequeueAfter(5 * time.Second),
				publicKey:     "",
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionFalse),
			},
		},
		{
			name: "invalid PEM — sets False and requeues",
			env: env{
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return []byte("not-pem-data"), nil
				},
			},
			want: want{
				result:        testAction.RequeueAfter(5 * time.Second),
				publicKey:     "",
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionFalse),
			},
		},
		{
			name: "transient error preserves existing key",
			env: env{
				publicKey: validPEM,
				conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					{Type: TrustMaterialCondition, Status: metav1.ConditionTrue, Reason: ReasonResolved,
						LastTransitionTime: metav1.Now()},
				},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("temporary network issue")
				},
			},
			want: want{
				result:        testAction.RequeueAfter(5 * time.Second),
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    condStatusPtr(metav1.ConditionFalse),
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
				publicKey:     validPEM,
				hasError:      true,
				resolveCalled: true,
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
				publicKey:     validPEM,
				hasError:      true,
				resolveCalled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			instance := newTestRekor(tt.env.publicKey, tt.env.conditions...)

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance)
			if tt.env.intercept.SubResourceUpdate != nil {
				builder = builder.WithInterceptorFuncs(tt.env.intercept)
			}
			c := builder.Build()

			resolver := &testResolver{resolve: tt.env.resolve}
			a := testAction.PrepareAction(c, NewAction[*rhtasv1.Rekor](resolver))
			result := a.Handle(t.Context(), instance)

			g.Expect(resolver.called).To(Equal(tt.want.resolveCalled), "Resolve() call expectation mismatch")

			if tt.want.hasError {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Err).To(HaveOccurred())
				g.Expect(errors.Is(result.Err, ErrPersistStatus)).To(BeTrue())
			} else if tt.want.result != nil {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Result.RequeueAfter).To(Equal(tt.want.result.Result.RequeueAfter))
			}

			g.Expect(instance.Status.PublicKey).To(Equal(tt.want.publicKey))

			if tt.want.condStatus != nil {
				cond := meta.FindStatusCondition(instance.GetConditions(), TrustMaterialCondition)
				g.Expect(cond).ToNot(BeNil(), "expected TrustMaterialAvailable condition")
				g.Expect(cond.Status).To(Equal(*tt.want.condStatus))
			}
		})
	}
}

func condStatusPtr(s metav1.ConditionStatus) *metav1.ConditionStatus {
	return &s
}

// newDriftTestRekor builds a Rekor with explicit annotations and conditions,
// for drift/debounce/acknowledgement scenarios that need control over Ready
// and the object's annotation map (unlike newTestRekor, which always sets a
// default Ready/Initialize condition and no annotations).
func newDriftTestRekor(publicKey string, anns map[string]string, conditions ...metav1.Condition) *rhtasv1.Rekor {
	return &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rekor", Namespace: "default", Annotations: anns},
		Status: rhtasv1.RekorStatus{
			Url:        "http://rekor-server.default.svc",
			PublicKey:  publicKey,
			Conditions: conditions,
		},
	}
}

// prepareActionWithRecorder mirrors testAction.PrepareAction but keeps a
// reference to the FakeRecorder so tests can assert on emitted events.
func prepareActionWithRecorder(c client.Client, resolver Resolver[*rhtasv1.Rekor]) (action.Action[*rhtasv1.Rekor], *events.FakeRecorder) {
	a := NewAction[*rhtasv1.Rekor](resolver)
	rec := events.NewFakeRecorder(10)
	a.InjectClient(c)
	a.InjectLogger(logr.Logger{})
	a.InjectRecorder(rec)
	return a, rec
}

func fetchRekor(t *testing.T, c client.Client, instance *rhtasv1.Rekor) *rhtasv1.Rekor {
	t.Helper()
	var fresh rhtasv1.Rekor
	if err := c.Get(t.Context(), client.ObjectKeyFromObject(instance), &fresh); err != nil {
		t.Fatalf("failed to fetch rekor: %v", err)
	}
	return &fresh
}

const oldPEM = "-----BEGIN PUBLIC KEY-----\nOLDKEY\n-----END PUBLIC KEY-----\n"

func TestResolvePubKey_Drift(t *testing.T) {
	t.Parallel()

	type env struct {
		publicKey   string
		annotations map[string]string
		conditions  []metav1.Condition
		resolve     func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error)
		intercept   interceptor.Funcs
	}
	type want struct {
		result            *action.Result
		publicKey         string
		resolveCalled     bool
		condStatus        metav1.ConditionStatus
		condReason        string
		condMessage       string // checked only if non-empty
		readyStatus       metav1.ConditionStatus
		readyReason       string
		event             string // substring expected; empty means no event fired
		annotationCleared *bool  // nil: no annotation set; else expected present(false)/absent(true) after Handle
	}

	readyCond := func(status metav1.ConditionStatus, message string) metav1.Condition {
		return metav1.Condition{Type: constants.ReadyCondition, Status: status, Reason: state.Ready.String(), Message: message}
	}
	trustMaterialCond := func(status metav1.ConditionStatus, reason, message string) metav1.Condition {
		return metav1.Condition{Type: TrustMaterialCondition, Status: status, Reason: reason, Message: message, LastTransitionTime: metav1.Now()}
	}
	resolvesTo := func(pem string) func(context.Context, client.Client, *rhtasv1.Rekor) ([]byte, error) {
		return func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) { return []byte(pem), nil }
	}

	const driftMessage = "original drifted message"

	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "new drift detected immediately — flags Drifted, blocks Ready, fires Warning once",
			env: env{
				publicKey:  oldPEM,
				conditions: []metav1.Condition{readyCond(metav1.ConditionTrue, ""), trustMaterialCond(metav1.ConditionTrue, ReasonResolved, "")},
				resolve:    resolvesTo(validPEM),
			},
			want: want{
				result:        testAction.Return(),
				publicKey:     oldPEM,
				resolveCalled: true,
				condStatus:    metav1.ConditionFalse,
				condReason:    ReasonDrifted,
				readyStatus:   metav1.ConditionFalse,
				readyReason:   state.Ready.String(),
				event:         "TrustMaterialDrifted",
			},
		},
		{
			name: "reverting to the recorded value self-heals",
			env: env{
				publicKey:  validPEM,
				conditions: []metav1.Condition{readyCond(metav1.ConditionFalse, "drifted"), trustMaterialCond(metav1.ConditionFalse, ReasonDrifted, "")},
				resolve:    resolvesTo(validPEM),
			},
			want: want{
				result:        testAction.Continue(),
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    metav1.ConditionTrue,
				condReason:    ReasonResolved,
				readyStatus:   metav1.ConditionFalse,
				readyReason:   state.Ready.String(),
			},
		},
		{
			name: "already-flagged drift always re-fetches and re-affirms without duplicating the event",
			env: env{
				publicKey:  oldPEM,
				conditions: []metav1.Condition{readyCond(metav1.ConditionFalse, "drifted"), trustMaterialCond(metav1.ConditionFalse, ReasonDrifted, "")},
				resolve:    resolvesTo(validPEM), // still different from oldPEM
			},
			want: want{
				result:        testAction.Return(),
				publicKey:     oldPEM,
				resolveCalled: true,
				condStatus:    metav1.ConditionFalse,
				condReason:    ReasonDrifted,
				readyStatus:   metav1.ConditionFalse,
				readyReason:   state.Ready.String(),
			},
		},
		{
			name: "acknowledging a real drift accepts it immediately regardless of Ready",
			env: env{
				publicKey:   oldPEM,
				annotations: map[string]string{annotations.RefreshTrustMaterial: "true"},
				conditions:  []metav1.Condition{readyCond(metav1.ConditionFalse, "drifted"), trustMaterialCond(metav1.ConditionFalse, ReasonDrifted, "")},
				resolve:     resolvesTo(validPEM),
			},
			want: want{
				result:            testAction.Continue(),
				publicKey:         validPEM,
				resolveCalled:     true,
				condStatus:        metav1.ConditionTrue,
				condReason:        ReasonResolved,
				readyStatus:       metav1.ConditionFalse,
				readyReason:       state.Ready.String(),
				event:             "TrustMaterialUpdated",
				annotationCleared: ptr.To(true),
			},
		},
		{
			name: "transient fetch error while already Drifted preserves the marker",
			env: env{
				publicKey:  oldPEM,
				conditions: []metav1.Condition{readyCond(metav1.ConditionFalse, "drifted"), trustMaterialCond(metav1.ConditionFalse, ReasonDrifted, driftMessage)},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("connection refused")
				},
			},
			want: want{
				result:        testAction.RequeueAfter(5 * time.Second),
				publicKey:     oldPEM,
				resolveCalled: true,
				condStatus:    metav1.ConditionFalse,
				condReason:    ReasonDrifted,
				condMessage:   driftMessage,
				readyStatus:   metav1.ConditionFalse,
				readyReason:   state.Ready.String(),
			},
		},
		{
			name: "transient fetch error unrelated to any drift leaves Ready untouched",
			env: env{
				publicKey:  validPEM,
				conditions: []metav1.Condition{readyCond(metav1.ConditionTrue, ""), trustMaterialCond(metav1.ConditionTrue, ReasonResolved, "")},
				resolve: func(_ context.Context, _ client.Client, _ *rhtasv1.Rekor) ([]byte, error) {
					return nil, errors.New("connection refused")
				},
			},
			want: want{
				result:        testAction.RequeueAfter(5 * time.Second),
				publicKey:     validPEM,
				resolveCalled: true,
				condStatus:    metav1.ConditionFalse,
				condReason:    ReasonResolveFailed,
				readyStatus:   metav1.ConditionTrue,
				readyReason:   state.Ready.String(),
			},
		},
		{
			name: "acknowledgement with no real drift is silently cleared, no bogus event",
			env: env{
				publicKey:   validPEM,
				annotations: map[string]string{annotations.RefreshTrustMaterial: "true"},
				conditions:  []metav1.Condition{readyCond(metav1.ConditionTrue, ""), trustMaterialCond(metav1.ConditionTrue, ReasonResolved, "")},
				resolve:     resolvesTo(validPEM), // unchanged
			},
			want: want{
				result:            testAction.Continue(),
				publicKey:         validPEM,
				resolveCalled:     true,
				condStatus:        metav1.ConditionTrue,
				condReason:        ReasonResolved,
				readyStatus:       metav1.ConditionTrue,
				readyReason:       state.Ready.String(),
				annotationCleared: ptr.To(true),
			},
		},
		{
			name: "annotation-clear failure after accepting is non-fatal",
			env: env{
				publicKey:   validPEM,
				annotations: map[string]string{annotations.RefreshTrustMaterial: "true"},
				conditions:  []metav1.Condition{readyCond(metav1.ConditionFalse, ""), trustMaterialCond(metav1.ConditionFalse, ReasonDrifted, "")},
				resolve:     resolvesTo(validPEM),
				intercept: interceptor.Funcs{
					Update: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.UpdateOption) error {
						return apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
					},
				},
			},
			want: want{
				result:            testAction.Continue(),
				publicKey:         validPEM,
				resolveCalled:     true,
				condStatus:        metav1.ConditionTrue,
				condReason:        ReasonResolved,
				readyStatus:       metav1.ConditionFalse,
				readyReason:       state.Ready.String(),
				event:             "AnnotationClearFailed",
				annotationCleared: ptr.To(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			instance := newDriftTestRekor(tt.env.publicKey, tt.env.annotations, tt.env.conditions...)

			builder := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance)
			if tt.env.intercept.Update != nil || tt.env.intercept.SubResourceUpdate != nil || tt.env.intercept.Patch != nil {
				builder = builder.WithInterceptorFuncs(tt.env.intercept)
			}
			c := builder.Build()

			resolver := &testResolver{resolve: tt.env.resolve}
			a, rec := prepareActionWithRecorder(c, resolver)

			result := a.Handle(t.Context(), instance)

			g.Expect(resolver.called).To(Equal(tt.want.resolveCalled), "Resolve() call expectation mismatch")

			if tt.want.result == nil {
				g.Expect(result).To(BeNil(), "expected Continue()")
			} else {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Result.RequeueAfter).To(Equal(tt.want.result.Result.RequeueAfter))
			}

			g.Expect(instance.Status.PublicKey).To(Equal(tt.want.publicKey))

			cond := meta.FindStatusCondition(instance.GetConditions(), TrustMaterialCondition)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.Status).To(Equal(tt.want.condStatus))
			g.Expect(cond.Reason).To(Equal(tt.want.condReason))
			if tt.want.condMessage != "" {
				g.Expect(cond.Message).To(Equal(tt.want.condMessage))
			}

			ready := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
			g.Expect(ready).ToNot(BeNil())
			g.Expect(ready.Status).To(Equal(tt.want.readyStatus))
			g.Expect(ready.Reason).To(Equal(tt.want.readyReason))

			if tt.want.event != "" {
				select {
				case ev := <-rec.Events:
					g.Expect(ev).To(ContainSubstring(tt.want.event))
				default:
					t.Fatal("expected an event but none was recorded")
				}
			} else {
				g.Expect(rec.Events).To(BeEmpty())
			}

			if tt.want.annotationCleared != nil {
				fresh := fetchRekor(t, c, instance)
				if *tt.want.annotationCleared {
					g.Expect(fresh.Annotations).ToNot(HaveKey(annotations.RefreshTrustMaterial))
				} else {
					g.Expect(fresh.Annotations).To(HaveKey(annotations.RefreshTrustMaterial))
				}
			}
		})
	}
}
