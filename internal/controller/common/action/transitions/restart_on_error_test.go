package transitions

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_HandleError(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: v1.ObjectMeta{Name: "error", Namespace: "default"},
		Status: v1alpha1.CTlogStatus{
			RecoveryAttempts: 0,
			Conditions: []v1.Condition{{
				Type:   constants.Ready,
				Status: v1.ConditionFalse,
				Reason: constants.Error,
			}},
		}}

	a := NewRestartOnErrorAction[*v1alpha1.CTlog]()
	f := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a = testAction.PrepareAction(f, a)

	ctx := context.TODO()
	g.Expect(a.CanHandleError(ctx, instance)).To(BeTrue())
	result := a.HandleError(ctx, instance)

	g.Expect(result).Should(Equal(testAction.StatusUpdate()))
	g.Expect(instance.Status.RecoveryAttempts).Should(Equal(int64(1)))
	g.Expect(meta.FindStatusCondition(instance.GetConditions(), constants.Ready).Reason).Should(Equal(constants.Pending))
}

func Test_HandleError_Threshold(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: v1.ObjectMeta{Name: "treshold", Namespace: "default"},
		Status: v1alpha1.CTlogStatus{
			Conditions: []v1.Condition{{
				Type:   constants.Ready,
				Status: v1.ConditionFalse,
				Reason: constants.Error,
			}},
			RecoveryAttempts: constants.AllowedRecoveryAttempts - 1,
		}}

	a := NewRestartOnErrorAction[*v1alpha1.CTlog]()
	f := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()

	a = testAction.PrepareAction(f, a)
	ctx := context.TODO()
	g.Expect(a.CanHandleError(ctx, instance)).To(BeTrue())
	result := a.HandleError(ctx, instance)

	g.Expect(result).Should(Equal(testAction.FailWithStatusUpdate(fmt.Errorf("error"))))
	g.Expect(instance.Status.RecoveryAttempts).Should(Equal(constants.AllowedRecoveryAttempts))
	g.Expect(meta.FindStatusCondition(instance.GetConditions(), constants.Ready).Reason).Should(Equal(constants.Failure))
}

func Test_HandleError_Running(t *testing.T) {
	g := NewWithT(t)

	instance := &v1alpha1.CTlog{
		ObjectMeta: v1.ObjectMeta{Name: "handleRunning", Namespace: "default"},
		Status: v1alpha1.CTlogStatus{
			Conditions: []v1.Condition{{
				Type:   constants.Ready,
				Status: v1.ConditionTrue,
				Reason: constants.Ready,
			}},
			RecoveryAttempts: 2,
		}}

	a := NewRestartOnErrorAction[*v1alpha1.CTlog]()
	f := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a = testAction.PrepareAction(f, a)
	ctx := context.TODO()
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())
	result := a.Handle(ctx, instance)

	g.Expect(result).Should(Equal(testAction.StatusUpdate()))
	g.Expect(instance.Status.RecoveryAttempts).Should(Equal(int64(0)))
	g.Expect(meta.FindStatusCondition(instance.GetConditions(), constants.Ready).Reason).Should(Equal(constants.Ready))
}
