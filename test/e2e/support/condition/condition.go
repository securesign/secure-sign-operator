package condition

import (
	"context"
	"strings"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsReady(f apis.ConditionsAwareObject) bool {
	if f == nil {
		return false
	}
	return meta.IsStatusConditionTrue(f.GetConditions(), constants.ReadyCondition)
}

// IsReadyForGeneration reports whether f is Ready for its current generation.
// Unlike IsReady it rejects a verdict the controller recorded for an earlier
// generation.
func IsReadyForGeneration(f apis.ConditionsAwareObject) bool {
	if f == nil {
		return false
	}
	c := meta.FindStatusCondition(f.GetConditions(), constants.ReadyCondition)
	return c != nil && c.Status == metav1.ConditionTrue && c.ObservedGeneration == f.GetGeneration()
}

// HasFailureForGeneration reports whether conditionType is False for f's current
// generation and its message contains substr. Unlike waiting for Ready to time
// out, it distinguishes a deliberate refusal from a component that is slow.
func HasFailureForGeneration(f apis.ConditionsAwareObject, conditionType, substr string) bool {
	if f == nil {
		return false
	}
	c := meta.FindStatusCondition(f.GetConditions(), conditionType)
	return c != nil && c.Status == metav1.ConditionFalse &&
		c.ObservedGeneration == f.GetGeneration() && strings.Contains(c.Message, substr)
}

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace, component string) (bool, error) {
	return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
		labels.LabelAppPartOf:    constants.AppName,
		labels.LabelAppComponent: component,
	})
}
