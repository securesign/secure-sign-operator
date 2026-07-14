package deploymentRollout

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const testDeploymentName = "test-deployment"

func TestRolloutCheck_CanHandle(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		enabled func(*rhtasv1.CTlog) bool
		want    bool
	}{
		{name: "Pending is below threshold", reason: state.Pending.String(), want: false},
		{name: "Creating is below threshold", reason: state.Creating.String(), want: false},
		{name: "Initialize reaches threshold", reason: state.Initialize.String(), want: true},
		{name: "Ready is above threshold (the fix)", reason: state.Ready.String(), want: true},
		{name: "Failure must never be clobbered", reason: state.Failure.String(), want: false},
		{name: "missing condition", reason: "", want: false},
		{name: "disabled overrides an otherwise-true state", reason: state.Ready.String(), enabled: func(*rhtasv1.CTlog) bool { return false }, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			instance := &rhtasv1.CTlog{}
			if tt.reason != "" {
				instance.SetCondition(metav1.Condition{
					Type:   constants.ReadyCondition,
					Status: metav1.ConditionFalse,
					Reason: tt.reason,
				})
			}
			a := rolloutCheck[*rhtasv1.CTlog]{cfg: Config[*rhtasv1.CTlog]{
				ConditionType: constants.ReadyCondition,
				Enabled:       tt.enabled,
			}}
			g.Expect(a.CanHandle(t.Context(), instance)).To(gomega.Equal(tt.want))
		})
	}
}

func rolledOutDeployment() *appsv1.Deployment {
	return k8sTest.RolledOutDeployment(testDeploymentName, "ns")
}

func notRolledOutDeployment() *appsv1.Deployment {
	return k8sTest.StalledDeployment(testDeploymentName, "ns")
}

const testSubCondition = "SubCondition"

func TestRolloutCheck_Handle(t *testing.T) {
	errTransient := errors.New("api server unavailable")

	tests := []struct {
		name             string
		conditionType    string
		conditions       []metav1.Condition
		deployment       *appsv1.Deployment
		intercept        interceptor.Funcs
		promoteOnSuccess bool
		wantResult       *action.Result
		verify           func(g gomega.Gomega, updated *rhtasv1.CTlog)
	}{
		{
			name:          "a transient, non-rollout error is propagated without touching status",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
			intercept: interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, isDeployment := obj.(*appsv1.Deployment); isDeployment {
						return errTransient
					}
					return c.Get(ctx, key, obj, opts...)
				},
			},
			wantResult: testAction.Error(errTransient),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Reason).To(gomega.Equal(state.Initialize.String()), "status must be untouched by a transient error")
			},
		},
		{
			name:          "deployment missing requeues and sets the real error as message",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
			deployment: nil,
			wantResult: testAction.RequeueAfter(5 * time.Second),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(gomega.Equal(state.Initialize.String()))
				g.Expect(cond.Message).ToNot(gomega.BeEmpty())
				g.Expect(cond.Message).ToNot(gomega.Equal("Waiting for deployment to be ready"))
			},
		},
		{
			name:          "regresses an already-True condition when the deployment is no longer rolled out",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
			deployment: notRolledOutDeployment(),
			wantResult: testAction.RequeueAfter(5 * time.Second),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(gomega.Equal(state.Initialize.String()))
			},
		},
		{
			name:          "promotes to True/Ready when running and PromoteOnSuccess is set",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
			deployment:       rolledOutDeployment(),
			promoteOnSuccess: true,
			wantResult:       testAction.Return(),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(gomega.Equal(state.Ready.String()))
			},
		},
		{
			name:          "no-ops when already True/Ready and still running",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
			deployment:       rolledOutDeployment(),
			promoteOnSuccess: true,
			wantResult:       testAction.Continue(),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(gomega.Equal(state.Ready.String()))
			},
		},
		{
			name:          "continues without touching the condition when PromoteOnSuccess is false",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
			},
			deployment: rolledOutDeployment(),
			wantResult: testAction.Continue(),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(gomega.Equal(state.Initialize.String()))
			},
		},
		{
			name:          "promotes a distinct sub-condition without touching the main Ready condition",
			conditionType: testSubCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
				{Type: testSubCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
			},
			deployment:       rolledOutDeployment(),
			promoteOnSuccess: true,
			wantResult:       testAction.Return(),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				sub := meta.FindStatusCondition(updated.GetConditions(), testSubCondition)
				g.Expect(sub.Status).To(gomega.Equal(metav1.ConditionTrue))
				g.Expect(sub.Reason).To(gomega.Equal(state.Ready.String()))

				main := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(main.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(main.Reason).To(gomega.Equal(state.Initialize.String()))
			},
		},
		{
			name:          "a distinct sub-condition regressing after full Ready also demotes the main Ready condition",
			conditionType: testSubCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				{Type: testSubCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
			deployment:       notRolledOutDeployment(),
			promoteOnSuccess: true,
			wantResult:       testAction.RequeueAfter(5 * time.Second),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				sub := meta.FindStatusCondition(updated.GetConditions(), testSubCondition)
				g.Expect(sub.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(sub.Reason).To(gomega.Equal(state.Initialize.String()))

				main := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(main.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(main.Reason).To(gomega.Equal(state.Initialize.String()))
				g.Expect(main.Message).To(gomega.Equal("Waiting for " + testDeploymentName))
			},
		},
		{
			name:          "single-deployment family (ConditionType == Ready) never double-writes on regression",
			conditionType: constants.ReadyCondition,
			conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
			},
			deployment: notRolledOutDeployment(),
			wantResult: testAction.RequeueAfter(5 * time.Second),
			verify: func(g gomega.Gomega, updated *rhtasv1.CTlog) {
				g.Expect(updated.GetConditions()).To(gomega.HaveLen(1), "no duplicate condition entry should be created")
				cond := meta.FindStatusCondition(updated.GetConditions(), constants.ReadyCondition)
				g.Expect(cond.Status).To(gomega.Equal(metav1.ConditionFalse))
				g.Expect(cond.Reason).To(gomega.Equal(state.Initialize.String()))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			instance := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
				Status:     rhtasv1.CTlogStatus{Conditions: tt.conditions},
			}
			objs := []client.Object{instance}
			if tt.deployment != nil {
				objs = append(objs, tt.deployment)
			}

			c := testAction.FakeClientBuilder().WithObjects(objs...).WithStatusSubresource(instance).WithInterceptorFuncs(tt.intercept).Build()
			a := testAction.PrepareAction(c, NewAction(Config[*rhtasv1.CTlog]{
				ConditionType:    tt.conditionType,
				DeploymentName:   testDeploymentName,
				PromoteOnSuccess: tt.promoteOnSuccess,
			}))

			result := a.Handle(t.Context(), instance)

			g.Expect(result).To(gomega.Equal(tt.wantResult))

			updated := &rhtasv1.CTlog{}
			g.Expect(c.Get(t.Context(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated)).To(gomega.Succeed())
			tt.verify(g, updated)
		})
	}
}
