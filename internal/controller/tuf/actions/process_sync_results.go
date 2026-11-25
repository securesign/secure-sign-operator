package actions

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/traits"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewProcessSyncResultsAction() action.Action[*rhtasv1.Tuf] {
	return &processSyncResultsAction{}
}

type processSyncResultsAction struct {
	action.BaseAction
}

func (i processSyncResultsAction) Name() string {
	return "process sync job results"
}

func (i processSyncResultsAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, tufConstants.TrustRootSynced)
	return c != nil && c.Status == metav1.ConditionFalse && c.Reason == tufConstants.SyncInProgress
}

func (i processSyncResultsAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	j, err := job.GetJob(ctx, i.Client, instance.Namespace, syncJobName)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	switch {
	case !job.IsCompleted(*j):
		return i.RequeueAfter(5 * time.Second)
	case job.IsFailed(*j):
		// TODO: what to do when the sync job fails?
		return i.Error(ctx, fmt.Errorf("sync job failed"), instance)
	default:
		observedGeneration, err := strconv.ParseInt(j.Annotations[tufObservedGenerationAnnotation], 10, 64)
		if err != nil {
			i.Recorder.Eventf(instance, j, v1.EventTypeWarning, "TrustRootSyncFailure", "TufObservedGeneration", fmt.Sprintf("%s cannot be parsed to int - remove Job and retry", tufObservedGenerationAnnotation))
			i.Client.Delete(ctx, j)
			return i.Error(ctx, fmt.Errorf("failed to parse observed generation: %w", err), instance)
		}

		desired, ok := j.Annotations[trustRootDesiredAnnotation]
		if !ok {
			i.Recorder.Eventf(instance, j, v1.EventTypeWarning, "TrustRootSyncFailure", "TrustRootDesiredAnnotationNotFound", fmt.Sprintf("%s annotation not found - remove Job and retry", trustRootDesiredAnnotation))
			i.Client.Delete(ctx, j)
			return i.Error(ctx, fmt.Errorf("trust root desired annotation not found"), instance)
		}

		components := &rhtasv1.Components{}
		if err := json.Unmarshal([]byte(desired), components); err != nil {
			i.Recorder.Eventf(instance, j, v1.EventTypeWarning, "TrustRootSyncFailure", "Failed to unmarshal trust root desired annotation", err.Error())
			i.Client.Delete(ctx, j)
			return i.Error(ctx, fmt.Errorf("failed to unmarshal trust root desired annotation: %w", err), instance)
		}

		// go limitation - can't for cycle over trustroot.TrustRootBinder interface so we need to materialize bindings for each component type
		if err := materializeBindings(ctx, i.Client, instance, trustroot.CTFE, components.Ctlog); err != nil {
			return i.Error(ctx, fmt.Errorf("failed to materialize ctlog bindings: %w", err), instance)
		}
		if err := materializeBindings(ctx, i.Client, instance, trustroot.Rekor, components.Rekor); err != nil {
			return i.Error(ctx, fmt.Errorf("failed to materialize rekor bindings: %w", err), instance)
		}
		if err := materializeBindings(ctx, i.Client, instance, trustroot.Fulcio, components.Fulcio); err != nil {
			return i.Error(ctx, fmt.Errorf("failed to materialize fulcio bindings: %w", err), instance)
		}
		if err := materializeBindings(ctx, i.Client, instance, trustroot.TSA, components.Tsa); err != nil {
			return i.Error(ctx, fmt.Errorf("failed to materialize bindings: %w", err), instance)
		}

		instance.Status.TrustRoot.Published = components
		instance.Status.TrustRoot.LastPublishTime = j.Status.CompletionTime
		// TODO: set PublishedTargetsVersion
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               tufConstants.TrustRootSynced,
			Status:             metav1.ConditionTrue,
			Reason:             tufConstants.Synchronized,
			Message:            fmt.Sprintf("Trust root synchronized by job: %s", j.Name),
			ObservedGeneration: observedGeneration,
		})

		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

}

func materializeBindings[T trustroot.TrustRootBinder](ctx context.Context, c client.Client, instance *rhtasv1.Tuf, component trustroot.ComponentKey, components []T) error {
	for i, binding := range components {
		if binding.GetServiceRef().Ref == nil || binding.GetServiceRef().Ref.Name == "" {
			continue
		}

		resolved, err := trustroot.ResolveBinding(ctx, c, instance.Namespace, component, binding)
		if err != nil {
			return fmt.Errorf("resolve binding %T: %w", binding, err)
		}
		target := cmp.Or(binding.GetName(), string(component))
		secret, err := traits.Apply(coreac.Secret(target, instance.Namespace).WithImmutable(true).WithData(map[string][]byte{
			target: []byte(resolved.TrustMaterial),
		}),
			traits.Labels[*coreac.SecretApplyConfiguration](labels.ForComponent(tufConstants.ComponentName, instance.Name)),
		)
		if err != nil {
			return fmt.Errorf("apply secret %T: %w", binding, err)
		}

		if err := c.Apply(ctx, secret); err != nil {
			return fmt.Errorf("apply secret %T: %w", binding, err)
		}

		newBinding := rhtasv1.TrustRootBinding{
			SecretRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: target,
				},
				Key: target,
			},
			ServiceReference: rhtasv1.ServiceReference{
				URL: resolved.BaseURL,
			},
			ValidFor:   binding.GetValidFor().DeepCopy(),
			TargetName: target,
		}

		if c, ok := any(binding).(rhtasv1.TrustRootBindingWithOIDC); ok {
			newWithOIDC := rhtasv1.TrustRootBindingWithOIDC{
				TrustRootBinding: newBinding,
				OIDCIssuers:      c.OIDCIssuers,
			}
			components[i] = any(newWithOIDC).(T)
		} else {
			components[i] = any(newBinding).(T)
		}
	}
	return nil
}
