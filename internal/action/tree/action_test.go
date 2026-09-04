package tree

import (
	"context"
	stderrors "errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

type namedTest struct {
	name string
	run  func(t *testing.T)
}

var tests []namedTest

var defaultWrapper = Wrapper[*rhtasv1.Rekor](
	func(rekor *rhtasv1.Rekor) *int64 {
		return rekor.Spec.TreeID
	},
	func(rekor *rhtasv1.Rekor) *int64 {
		return rekor.Status.TreeID
	},
	func(rekor *rhtasv1.Rekor, i *int64) {
		rekor.Status.TreeID = i
	},
	func(rekor *rhtasv1.Rekor) *rhtasv1.ServiceReference {
		return &rhtasv1.ServiceReference{
			URL: "https://trillian-logserver.default.svc",
		}
	},
)

var (
	nnObject = types.NamespacedName{Name: "test", Namespace: "default"}
	nnResult = types.NamespacedName{Name: fmt.Sprintf(configMapResultMask, "test", "test"), Namespace: "default"}
)

func init() {
	tests = []namedTest{
		{name: "missingCondition", run: testMissingCondition},
		{name: "manual", run: testManual},
		{name: "rbac", run: testRbac},
		{name: "configmap", run: testConfigMap},
		{name: "create-job", run: testCreateJob},
		{name: "monitor-job", run: testMonitorJob},
		{name: "extract-result", run: testExtractResult},
	}
}

type pre struct {
	warmUp bool
	before func(context.Context, Gomega, client.WithWatch)
}
type want struct {
	result *action.Result
	verify func(context.Context, Gomega, client.WithWatch)
}

func testMissingCondition(t *testing.T) {
	for _, tc := range []struct {
		desc string
		want want
		pre  pre
	}{
		{
			desc: "treeID set, condition not-set",
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(r.GetConditions(), JobCondition)).To(BeTrue())
				},
			},
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					r.Status.TreeID = ptr.To(int64(123456789))
					g.Expect(c.Status().Update(ctx, &r)).To(Succeed())
				},
			},
		},
		{
			desc: "treeID not set, condition not-set",
			want: want{
				result: nil,
			},
			pre: pre{},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleMissingCondition(ctx, rekor)
		}))
	}
}

func testManual(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "not-set",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Spec.TreeID).Should(BeNil())
					g.Expect(r.Status.TreeID).Should(BeNil())
				},
			},
		},
		{
			desc: "set",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					r.Spec.TreeID = ptr.To(int64(123456789))
					g.Expect(c.Update(ctx, &r)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(r.Status.TreeID).ShouldNot(BeNil())
					g.Expect(*r.Spec.TreeID).Should(Equal(int64(123456789)))
					g.Expect(*r.Status.TreeID).Should(Equal(int64(123456789)))

					cond := meta.FindStatusCondition(r.GetConditions(), JobCondition)
					g.Expect(cond).Should(BeNil())
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleManual(ctx, rekor)
		}))
	}
}

func testRbac(t *testing.T) {
	for _, tc := range []struct {
		desc string
		want want
	}{
		{desc: "ensure", want: want{
			result: testAction.Continue(),
			verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
				nn := types.NamespacedName{Name: fmt.Sprintf(RBACNameMask, "test"), Namespace: "default"}
				g.Expect(c.Get(ctx, nn, &corev1.ServiceAccount{})).To(Succeed())
				g.Expect(c.Get(ctx, nn, &rbacv1.Role{})).To(Succeed())
				g.Expect(c.Get(ctx, nn, &rbacv1.RoleBinding{})).To(Succeed())
			},
		}},
	} {
		t.Run(tc.desc, testRunner(pre{}, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleRbac(ctx, rekor)
		}))
	}
}

func testConfigMap(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "not-exists",
			pre: pre{
				warmUp: false,
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, nnResult, &corev1.ConfigMap{})).To(Succeed())
				},
			}},
		{
			desc: "exists",
			pre: pre{
				warmUp: true,
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, nnResult, &corev1.ConfigMap{})).To(Succeed())
				},
			}},
		{
			desc: "ignore-changes-data",
			pre: pre{
				warmUp: true,
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())

					cm.Data = map[string]string{
						"foo": "bar",
					}
					g.Expect(c.Update(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())

					g.Expect(cm.Data["foo"]).To(Equal("bar"))
				},
			}},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleConfigMap(ctx, rekor)
		}))
	}
}

func testCreateJob(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue",
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "continue",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job-name"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
			},
		},
		{
			desc: "ensure",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					jobs := &v1.JobList{}
					g.Expect(c.List(ctx, jobs, client.InNamespace("default"))).To(Succeed())
					g.Expect(jobs.Items).To(HaveLen(1))
					jobName := jobs.Items[0].Name

					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())
					g.Expect(cm.GetOwnerReferences()).To(ContainElements(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Kind": Equal("Job"),
						"Name": Equal(jobName),
					})))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleJob(ctx, rekor)
		}))
	}
}

// TestResolveTree_BackoffGate: a future next-retry requeues without recreating the Job.
func TestResolveTree_BackoffGate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: nnObject.Name, Namespace: nnObject.Namespace},
		Spec:       rhtasv1.RekorSpec{Trillian: rhtasv1.ServiceReference{}},
	}
	generation := strconv.FormatInt(instance.GetGeneration(), 10)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nnResult.Name,
			Namespace: nnResult.Namespace,
			Annotations: map[string]string{
				annotations.CreateTreeAttempts:           "1",
				annotations.CreateTreeAttemptsGeneration: generation,
				annotations.CreateTreeNextRetry:          time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, cm).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
	ra := a.(*resolveTree[*rhtasv1.Rekor])

	got := ra.handleJob(ctx, instance)
	g.Expect(got).ToNot(BeNil())
	g.Expect(got.Result.RequeueAfter).To(BeNumerically(">", 0))

	jobs := &v1.JobList{}
	g.Expect(c.List(ctx, jobs, client.InNamespace(nnObject.Namespace))).To(Succeed())
	g.Expect(jobs.Items).To(BeEmpty())
}

// TestResolveTree_BackoffGateElapsed: a past next-retry recreates the Job.
func TestResolveTree_BackoffGateElapsed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{Name: nnObject.Name, Namespace: nnObject.Namespace},
		Spec:       rhtasv1.RekorSpec{Trillian: rhtasv1.ServiceReference{}},
	}
	generation := strconv.FormatInt(instance.GetGeneration(), 10)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nnResult.Name,
			Namespace: nnResult.Namespace,
			Annotations: map[string]string{
				annotations.CreateTreeAttempts:           "1",
				annotations.CreateTreeAttemptsGeneration: generation,
				annotations.CreateTreeNextRetry:          time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, cm).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
	ra := a.(*resolveTree[*rhtasv1.Rekor])

	g.Expect(ra.handleJob(ctx, instance)).To(Equal(testAction.Return()))

	jobs := &v1.JobList{}
	g.Expect(c.List(ctx, jobs, client.InNamespace(nnObject.Namespace))).To(Succeed())
	g.Expect(jobs.Items).To(HaveLen(1))
}

func testMonitorJob(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue: missing configmap",
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "requeue: missing reference",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "requeue: missing job",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "requeue: job not completed",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "job failed",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{
									Status: corev1.ConditionTrue, Type: v1.JobComplete,
								},
								{
									Status: corev1.ConditionTrue, Type: v1.JobFailed,
								},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				// first failure -> attempt 1, linear backoff base*1
				result: testAction.RequeueAfter(createTreeRetryBaseDelay),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					// failed Job is deleted so it can be recreated
					job := &v1.Job{}
					g.Expect(apierrors.IsNotFound(
						c.Get(ctx, types.NamespacedName{Name: "job", Namespace: nnResult.Namespace}, job),
					)).To(BeTrue())

					// Job owner reference is removed from the result ConfigMap so
					// handleJob recreates the Job on the next reconcile
					cm := &corev1.ConfigMap{}
					g.Expect(c.Get(ctx, nnResult, cm)).To(Succeed())
					for _, ref := range cm.GetOwnerReferences() {
						g.Expect(ref.Kind).ToNot(Equal("Job"))
					}

					// attempt count is recorded on the ConfigMap
					g.Expect(cm.GetAnnotations()).To(HaveKeyWithValue(annotations.CreateTreeAttempts, "1"))

					// the resource is not frozen into a terminal Failure state
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					cond := meta.FindStatusCondition(r.GetConditions(), JobCondition)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.Reason).ToNot(Equal(state.Failure.String()))
				},
			},
		},
		{
			desc: "job failed at max attempts is terminal and keeps the failed job",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
							Annotations: map[string]string{
								// one below the cap, recorded for the current generation
								annotations.CreateTreeAttempts:           strconv.Itoa(createTreeMaxAttempts - 1),
								annotations.CreateTreeAttemptsGeneration: strconv.FormatInt(r.GetGeneration(), 10),
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{Status: corev1.ConditionTrue, Type: v1.JobComplete},
								{Status: corev1.ConditionTrue, Type: v1.JobFailed},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(ErrJobFailed)),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					// the failed Job is kept for debugging
					job := &v1.Job{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: "job", Namespace: nnResult.Namespace}, job)).To(Succeed())

					// Tree condition is terminal Failure
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					cond := meta.FindStatusCondition(r.GetConditions(), JobCondition)
					g.Expect(cond).ToNot(BeNil())
					g.Expect(cond.Reason).To(Equal(state.Failure.String()))
				},
			},
		},
		{
			desc: "job failed but result present adopts existing tree",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
						Data: map[string]string{
							"tree_id": "123456789",
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())

					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{Status: corev1.ConditionTrue, Type: v1.JobComplete},
								{Status: corev1.ConditionTrue, Type: v1.JobFailed},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					// the Job is left intact for handleExtractJobResult to consume
					job := &v1.Job{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: "job", Namespace: nnResult.Namespace}, job)).To(Succeed())
				},
			},
		},
		{
			desc: "continue",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
					job := v1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "job",
							Namespace: nnResult.Namespace,
						},
						Status: v1.JobStatus{
							Conditions: []v1.JobCondition{
								{
									Status: corev1.ConditionTrue, Type: v1.JobComplete,
								},
							},
						},
					}
					g.Expect(c.Create(ctx, &job)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleJobFinished(ctx, rekor)
		}))
	}
}

func testExtractResult(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "requeue: missing configmap",
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "requeue: missing data",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.RequeueAfter(5 * time.Second),
			},
		},
		{
			desc: "error: corrupted data",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
						Data: map[string]string{
							"tree_id": "not-a-number",
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(
					&strconv.NumError{Func: "ParseInt", Num: "not-a-number", Err: strconv.ErrSyntax},
				)),
			},
		},
		{
			desc: "success: update TreeID status",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnResult.Name,
							Namespace: nnResult.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{Kind: "Job", Name: "job"},
							},
						},
						Data: map[string]string{
							"tree_id": "123456789",
						},
					}
					g.Expect(c.Create(ctx, cm)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					r := rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())

					g.Expect(r.Status.TreeID).ToNot(BeNil())
					g.Expect(*r.Status.TreeID).To(Equal(int64(123456789)))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *resolveTree[*rhtasv1.Rekor], ctx context.Context, rekor *rhtasv1.Rekor) *action.Result {
			return r.handleExtractJobResult(ctx, rekor)
		}))
	}
}

type handleFn func(*resolveTree[*rhtasv1.Rekor], context.Context, *rhtasv1.Rekor) *action.Result

func testRunner(pre pre, want want, handleFn handleFn) func(t *testing.T) {
	return func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		instance := &rhtasv1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nnObject.Name,
				Namespace: nnObject.Namespace,
			},
			Spec: rhtasv1.RekorSpec{
				Trillian: rhtasv1.ServiceReference{},
			},
		}

		c := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
		ra := a.(*resolveTree[*rhtasv1.Rekor])

		if pre.warmUp {
			handleFn(ra, ctx, instance)
		}

		if pre.before != nil {
			pre.before(ctx, g, c)
		}

		g.Expect(c.Get(ctx, nnObject, instance)).To(Succeed())

		if got := handleFn(ra, ctx, instance); !reflect.DeepEqual(got, want.result) {
			t.Errorf("CanHandle() = %v, want %v", got, want.result)
		}
		if want.verify != nil {
			want.verify(ctx, g, c)
		}
	}
}

func TestResolveTree_CanHandle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		condition    metav1.ConditionStatus
		canHandle    bool
		treeID       *int64
		statusTreeID *int64
	}{
		{
			name:      "spec.treeID is not nil and status.treeID is nil",
			condition: metav1.ConditionTrue,
			canHandle: true,
			treeID:    ptr.To(int64(123456)),
		}, {
			name:         "spec.treeID != status.treeID",
			condition:    metav1.ConditionTrue,
			canHandle:    true,
			treeID:       ptr.To(int64(123456)),
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "spec.treeID is nil and status.treeID is not nil",
			condition:    metav1.ConditionTrue,
			canHandle:    false,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:      "spec.treeID is nil and status.treeID is nil",
			condition: metav1.ConditionTrue,
			canHandle: true,
		}, {
			name:         "status condition is false",
			condition:    metav1.ConditionFalse,
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "status condition is Unknown",
			condition:    metav1.ConditionUnknown,
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		}, {
			name:         "empty status",
			condition:    "",
			canHandle:    true,
			statusTreeID: ptr.To(int64(654321)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
			instance := rhtasv1.Rekor{
				Spec: rhtasv1.RekorSpec{
					TreeID: tt.treeID,
				},
				Status: rhtasv1.RekorStatus{
					TreeID: tt.statusTreeID,
				},
			}
			if tt.condition != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   JobCondition,
					Status: tt.condition,
				})
			}

			if got := a.CanHandle(t.Context(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolveTree_Handle(t *testing.T) {
	for _, nt := range tests {
		t.Run(nt.name, func(t *testing.T) {
			nt.run(t)
		})
	}
}

func TestResolveTree_ConfigMapGetFailure_ReturnsRetryableError(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nnObject.Name,
			Namespace: nnObject.Namespace,
		},
		Spec: rhtasv1.RekorSpec{
			Trillian: rhtasv1.ServiceReference{},
		},
	}

	injectedErr := fmt.Errorf("connection refused")

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return injectedErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
	ra := a.(*resolveTree[*rhtasv1.Rekor])

	g.Expect(c.Get(ctx, nnObject, instance)).To(Succeed())

	for _, tc := range []struct {
		name     string
		handleFn func(context.Context, *rhtasv1.Rekor) *action.Result
	}{
		{name: "handleJob", handleFn: ra.handleJob},
		{name: "handleJobFinished", handleFn: ra.handleJobFinished},
		{name: "handleExtractJobResult", handleFn: ra.handleExtractJobResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			result := tc.handleFn(ctx, instance)
			g.Expect(result).ToNot(BeNil())
			g.Expect(result.Err).To(HaveOccurred())
			g.Expect(result.Err.Error()).To(ContainSubstring("could not get configmap"))
			g.Expect(stderrors.Is(result.Err, reconcile.TerminalError(nil))).To(BeFalse(),
				"ConfigMap Get errors must be retryable, not terminal")
		})
	}
}

func TestResolveTree_RbacCreationFailure_ReturnsRetryableError(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	instance := &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nnObject.Name,
			Namespace: nnObject.Namespace,
		},
		Spec: rhtasv1.RekorSpec{
			Trillian: rhtasv1.ServiceReference{},
		},
	}

	injectedErr := fmt.Errorf("connection refused")

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.ServiceAccount); ok {
					return injectedErr
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	a := testAction.PrepareAction(c, NewResolveTreeAction("test", defaultWrapper))
	ra := a.(*resolveTree[*rhtasv1.Rekor])

	g.Expect(c.Get(ctx, nnObject, instance)).To(Succeed())

	result := ra.handleRbac(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err.Error()).To(ContainSubstring("could not create SA"))
	g.Expect(stderrors.Is(result.Err, reconcile.TerminalError(nil))).To(BeFalse(),
		"API server errors must be retryable, not terminal")
}
