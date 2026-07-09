package monitoring

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	testComponent      = "test-component"
	testMonitoringRole = "prometheus-k8s-test"
	testServiceMonitor = "test-server"
	testCondition      = "TestCondition"
	testNamespace      = "test-ns"
	testInstanceName   = "test-instance"
)

var smGVK = schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor"}

type testConfig struct {
	enabled bool
	tls     rhtasv1.TLS
}

func (c testConfig) IsEnabled(*rhtasv1.Fulcio) bool  { return c.enabled }
func (c testConfig) TLS(*rhtasv1.Fulcio) rhtasv1.TLS { return c.tls }

func newTestInstance(conditions ...metav1.Condition) *rhtasv1.Fulcio {
	instance := &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name: testInstanceName, Namespace: testNamespace, Generation: 1,
		},
	}
	for _, c := range conditions {
		apimeta.SetStatusCondition(&instance.Status.Conditions, c)
	}
	return instance
}

func creatingConditions() []metav1.Condition {
	return []metav1.Condition{
		{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
	}
}

func noMatchInterceptor() interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if obj.GetObjectKind().GroupVersionKind().Kind == "ServiceMonitor" {
				return &apimeta.NoKindMatchError{
					GroupKind:        schema.GroupKind{Group: "monitoring.coreos.com", Kind: "ServiceMonitor"},
					SearchedVersions: []string{"v1"},
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}

func existingSM() *unstructured.Unstructured {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(smGVK)
	sm.SetName(testServiceMonitor)
	sm.SetNamespace(testNamespace)
	return sm
}

func getServiceMonitor(ctx context.Context, g Gomega, cli client.Client) *unstructured.Unstructured {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(smGVK)
	g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testServiceMonitor}, sm)).To(Succeed())
	return sm
}

func smEndpoints(g Gomega, sm *unstructured.Unstructured) []any {
	eps, found, err := unstructured.NestedSlice(sm.Object, "spec", "endpoints")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue(), "spec.endpoints not found")
	return eps
}

func smSelectorLabels(g Gomega, sm *unstructured.Unstructured) map[string]string {
	sel, found, err := unstructured.NestedStringMap(sm.Object, "spec", "selector", "matchLabels")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue(), "spec.selector.matchLabels not found")
	return sel
}

// ── CanHandle ───────────────────────────────────────────────────────────

func TestCanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  *rhtasv1.Fulcio
		canHandle bool
	}{
		{
			name:      "no conditions",
			instance:  newTestInstance(),
			canHandle: false,
		},
		{
			name: "Failure state",
			instance: newTestInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Failure.String(),
			}),
			canHandle: false,
		},
		{
			name: "Pending state",
			instance: newTestInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String(),
			}),
			canHandle: false,
		},
		{
			name:      "Creating state",
			instance:  newTestInstance(creatingConditions()...),
			canHandle: true,
		},
		{
			name: "Initialize state",
			instance: newTestInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String(),
			}),
			canHandle: true,
		},
		{
			name: "Ready state",
			instance: newTestInstance(metav1.Condition{
				Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String(),
			}),
			canHandle: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewAction(testComponent, testMonitoringRole, testServiceMonitor, "", testConfig{enabled: true}))
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

// ── Handle ──────────────────────────────────────────────────────────────

func TestHandle(t *testing.T) {
	g := NewWithT(t)

	tlsWithCert := rhtasv1.TLS{
		CertRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ca-secret"},
			Key:                  "ca.crt",
		},
	}

	type env struct {
		enabled       bool
		tls           rhtasv1.TLS
		conditionType string
		objects       []client.Object
		intercept     interceptor.Funcs
	}
	type want struct {
		result  *action.Result
		isError bool
		verify  func(context.Context, Gomega, *rhtasv1.Fulcio, client.WithWatch)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		// ── Enabled: HTTP ──────────────────────────────────────────────
		{
			name: "enabled HTTP — creates ServiceMonitor with correct spec",
			env:  env{enabled: true},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					sm := getServiceMonitor(ctx, g, cli)
					eps := smEndpoints(g, sm)
					g.Expect(eps).To(HaveLen(1))
					ep := eps[0].(map[string]any)
					g.Expect(ep["port"]).To(Equal("metrics"))
					g.Expect(ep["scheme"]).To(Equal("http"))
					g.Expect(ep["interval"]).To(Equal("30s"))
					g.Expect(ep).ToNot(HaveKey("tlsConfig"))

					g.Expect(smSelectorLabels(g, sm)).To(Equal(labels.ForComponent(testComponent, testInstanceName)))
					g.Expect(sm.GetLabels()).To(Equal(labels.For(testComponent, testMonitoringRole, testInstanceName)))

					ownerRefs := sm.GetOwnerReferences()
					g.Expect(ownerRefs).To(HaveLen(1))
					g.Expect(ownerRefs[0].Name).To(Equal(testInstanceName))
					g.Expect(*ownerRefs[0].Controller).To(BeTrue())
				},
			},
		},
		{
			name: "enabled HTTP — idempotent on second call",
			env:  env{enabled: true},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Fulcio, cli client.WithWatch) {
					a := testAction.PrepareAction(cli, NewAction(testComponent, testMonitoringRole, testServiceMonitor, "", testConfig{enabled: true}))
					g.Expect(a.Handle(ctx, instance)).To(BeNil())
					ep := smEndpoints(g, getServiceMonitor(ctx, g, cli))[0].(map[string]any)
					g.Expect(ep["port"]).To(Equal("metrics"))
					g.Expect(ep["scheme"]).To(Equal("http"))
				},
			},
		},
		{
			name: "enabled HTTP — corrects stale ServiceMonitor",
			env: env{
				enabled: true,
				objects: func() []client.Object {
					sm := existingSM()
					_ = unstructured.SetNestedSlice(sm.Object, []any{
						map[string]any{"port": "old-port", "scheme": "http"},
					}, "spec", "endpoints")
					return []client.Object{sm}
				}(),
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					ep := smEndpoints(g, getServiceMonitor(ctx, g, cli))[0].(map[string]any)
					g.Expect(ep["port"]).To(Equal("metrics"))
				},
			},
		},
		{
			name: "enabled HTTP — SM namespace matches instance",
			env:  env{enabled: true},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					g.Expect(getServiceMonitor(ctx, g, cli).GetNamespace()).To(Equal(testNamespace))
				},
			},
		},

		// ── Enabled: HTTPS/TLS ────────────────────────────────────────
		{
			name: "enabled HTTPS — creates ServiceMonitor with TLS config",
			env:  env{enabled: true, tls: tlsWithCert},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					sm := getServiceMonitor(ctx, g, cli)
					ep := smEndpoints(g, sm)[0].(map[string]any)
					g.Expect(ep["scheme"]).To(Equal("https"))

					tlsCfg, ok := ep["tlsConfig"].(map[string]any)
					g.Expect(ok).To(BeTrue())
					g.Expect(tlsCfg["serverName"]).To(Equal(fmt.Sprintf("%s.%s.svc", testServiceMonitor, testNamespace)))
					g.Expect(tlsCfg["insecureSkipVerify"]).To(BeFalse())

					ca := tlsCfg["ca"].(map[string]any)
					secret := ca["secret"].(map[string]any)
					g.Expect(secret["name"]).To(Equal("ca-secret"))
					g.Expect(secret["key"]).To(Equal("ca.crt"))
				},
			},
		},
		{
			name: "nil CertRef — falls back to HTTP",
			env:  env{enabled: true, tls: rhtasv1.TLS{CertRef: nil}},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					ep := smEndpoints(g, getServiceMonitor(ctx, g, cli))[0].(map[string]any)
					g.Expect(ep["scheme"]).To(Equal("http"))
					g.Expect(ep).ToNot(HaveKey("tlsConfig"))
				},
			},
		},

		// ── Disabled ──────────────────────────────────────────────────
		{
			name: "disabled — no SM exists, continues",
			env:  env{enabled: false},
			want: want{result: testAction.Continue()},
		},
		{
			name: "disabled — CRD not installed, continues",
			env: env{
				enabled: false,
				intercept: interceptor.Funcs{
					Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
						return &apimeta.NoKindMatchError{
							GroupKind:        schema.GroupKind{Group: "monitoring.coreos.com", Kind: "ServiceMonitor"},
							SearchedVersions: []string{"v1"},
						}
					},
				},
			},
			want: want{result: testAction.Continue()},
		},
		{
			name: "disabled — deletes existing SM",
			env:  env{enabled: false, objects: []client.Object{existingSM()}},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					sm := &unstructured.Unstructured{}
					sm.SetGroupVersionKind(smGVK)
					err := cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testServiceMonitor}, sm)
					g.Expect(err).To(HaveOccurred())
				},
			},
		},

		// ── API errors (retriable) ────────────────────────────────────
		{
			name: "create failure — retriable error",
			env: env{
				enabled: true,
				intercept: interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
						return fmt.Errorf("quota exceeded")
					},
				},
			},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Fulcio, cli client.WithWatch) {
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, instance)).To(Succeed())
				},
			},
		},
		{
			name: "create failure with condition — sets status condition",
			env: env{
				enabled:       true,
				conditionType: testCondition,
				intercept: interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
						return fmt.Errorf("quota exceeded")
					},
				},
			},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					fresh := &rhtasv1.Fulcio{}
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, fresh)).To(Succeed())
					cond := apimeta.FindStatusCondition(fresh.Status.Conditions, testCondition)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(cond.Reason).To(Equal(state.Failure.String()))
					g.Expect(cond.Message).To(ContainSubstring("quota exceeded"))

					readyCond := apimeta.FindStatusCondition(fresh.Status.Conditions, constants.ReadyCondition)
					g.Expect(readyCond).ToNot(BeNil())
					g.Expect(readyCond.Reason).ToNot(Equal(state.Failure.String()))
				},
			},
		},
		{
			name: "delete failure — retriable error",
			env: env{
				enabled: false,
				intercept: interceptor.Funcs{
					Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
						return fmt.Errorf("forbidden")
					},
				},
			},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Fulcio, cli client.WithWatch) {
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, instance)).To(Succeed())
				},
			},
		},

		// ── CRD missing ──────────────────────────────────────────────
		{
			name: "CRD missing — retriable error",
			env:  env{enabled: true, intercept: noMatchInterceptor()},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, instance *rhtasv1.Fulcio, cli client.WithWatch) {
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, instance)).To(Succeed())
				},
			},
		},
		{
			name: "CRD missing with condition — sets status condition",
			env:  env{enabled: true, conditionType: testCondition, intercept: noMatchInterceptor()},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					fresh := &rhtasv1.Fulcio{}
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, fresh)).To(Succeed())
					cond := apimeta.FindStatusCondition(fresh.Status.Conditions, testCondition)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.Message).To(ContainSubstring("ServiceMonitor CRD is not installed"))

					readyCond := apimeta.FindStatusCondition(fresh.Status.Conditions, constants.ReadyCondition)
					g.Expect(readyCond.Reason).ToNot(Equal(state.Failure.String()))
				},
			},
		},

		// ── No condition type — error without status condition ────────
		{
			name: "error without conditionType — no condition set",
			env: env{
				enabled: true,
				intercept: interceptor.Funcs{
					Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
						return fmt.Errorf("fail")
					},
				},
			},
			want: want{
				isError: true,
				verify: func(ctx context.Context, g Gomega, _ *rhtasv1.Fulcio, cli client.WithWatch) {
					fresh := &rhtasv1.Fulcio{}
					g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: testInstanceName}, fresh)).To(Succeed())
					cond := apimeta.FindStatusCondition(fresh.Status.Conditions, testCondition)
					g.Expect(cond).To(BeNil())
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			instance := newTestInstance(creatingConditions()...)

			builder := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				WithInterceptorFuncs(tt.env.intercept)
			for _, obj := range tt.env.objects {
				builder = builder.WithObjects(obj)
			}
			cli := builder.Build()

			a := testAction.PrepareAction(cli, NewAction(
				testComponent, testMonitoringRole, testServiceMonitor, tt.env.conditionType,
				testConfig{enabled: tt.env.enabled, tls: tt.env.tls},
			))

			got := a.Handle(ctx, instance)

			if tt.want.isError {
				if got == nil || got.Err == nil {
					t.Errorf("Handle() expected error, got %v", got)
				}
			} else if !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}

			if tt.want.verify != nil {
				tt.want.verify(ctx, g, instance, cli)
			}
		})
	}
}

func TestName(t *testing.T) {
	g := NewWithT(t)
	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewAction(testComponent, testMonitoringRole, testServiceMonitor, "", testConfig{enabled: true}))
	g.Expect(a.Name()).To(Equal("create monitoring"))
}
