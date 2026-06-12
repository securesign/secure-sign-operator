// Package action provides the framework for building reconciliation actions.
//
// # Overview
//
// Each controller reconciles a Custom Resource by running a sequence of actions.
// An action is a small, focused unit of work (create a Deployment, resolve a key,
// wait for readiness, etc.) that implements the [Action] interface.
//
// The reconciler executes actions in registration order:
//
//	for _, a := range actions {
//	    if a.CanHandle(ctx, instance) {
//	        result := a.Handle(ctx, instance)
//	        if result != nil {
//	            return result.Result, result.Err   // stop the chain
//	        }
//	    }
//	}
//	return reconcile.Result{}, nil
//
// # Flow Control
//
// Handle returns a *[Result] that tells the reconciler what to do next.
// There are exactly four outcomes:
//
//   - [BaseAction.Continue] (nil) — run the next action in the chain.
//   - [BaseAction.Return] — stop the chain; wait for a watch event to re-trigger.
//   - [BaseAction.Requeue] / [BaseAction.RequeueAfter] — stop the chain; re-trigger after a delay.
//   - [BaseAction.Error] — stop the chain; log the error, set failure conditions if terminal,
//     and let controller-runtime retry with exponential backoff.
//
// # Status Persistence
//
// [BaseAction.PersistStatus] writes the current in-memory status of the object
// to the API server. It returns (changed bool, err error) — it does NOT control
// flow. The changed flag indicates whether the status was actually written:
//
//   - changed=true: an API call was made and a watch event will fire, triggering
//     a new reconciliation. Use [BaseAction.Return] — the watch event handles it.
//   - changed=false: the status was already up-to-date, no API call was made,
//     and no watch event will fire. Use [BaseAction.Continue] to proceed to
//     the next action, since the status already reflects the desired state.
//
// [BaseAction.ReturnOnChange] is a convenience helper that encapsulates this
// logic. It accepts any func(context.Context, client.Object) → (bool, error)
// and returns a function that calls it and maps the result to the correct
// flow-control action:
//
//	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{...})
//	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
//
// When you need to run additional logic between PersistStatus and flow control
// (e.g. cleanup), use the expanded form:
//
//	changed, err := i.PersistStatus(ctx, instance)
//	if err != nil {
//	    return i.Error(ctx, err, instance)
//	}
//	i.cleanup(ctx, instance, labels)
//	if changed {
//	    return i.Return()
//	}
//	return i.Continue()
//
// # Implementing an Action
//
// Embed [BaseAction] and implement [Action]:
//
//	type myAction struct {
//	    action.BaseAction
//	}
//
//	func (a myAction) Name() string { return "my-action" }
//
//	func (a myAction) CanHandle(_ context.Context, instance *rhtasv1.MyResource) bool {
//	    // Gate on the state this action should handle.
//	    // This prevents re-entry when the status has already been set.
//	    return meta.IsStatusConditionFalse(instance.Status.Conditions, "Ready")
//	}
//
//	func (a myAction) Handle(ctx context.Context, instance *rhtasv1.MyResource) *action.Result {
//	    // Do work ...
//	    meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
//	        Type:   "Ready",
//	        Status: metav1.ConditionTrue,
//	        Reason: "Deployed",
//	    })
//	    return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
//	}
//
// # Common Patterns
//
// State transition (one-shot: set final state, let watch event drive next action):
//
//	meta.SetStatusCondition(&instance.Status.Conditions, readyCondition)
//	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
//
// Polling loop (wait for external state, retry periodically):
//
//	if !deploymentReady {
//	    meta.SetStatusCondition(&instance.Status.Conditions, waitingCondition)
//	    if _, err := i.PersistStatus(ctx, instance); err != nil {
//	        return i.Error(ctx, err, instance)
//	    }
//	    return i.RequeueAfter(5 * time.Second)
//	}
//
// Resource creation with status checkpoint:
//
//	if result != controllerutil.OperationResultNone {
//	    meta.SetStatusCondition(&instance.Status.Conditions, creatingCondition)
//	    return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
//	}
//	return i.Continue()
//
// Persist status then continue to next action.
// The changed flag can be safely ignored here because [BaseAction.Continue]
// proceeds to the next action in the same reconciliation cycle —
// no watch event is needed to drive progress:
//
//	meta.SetStatusCondition(&instance.Status.Conditions, intermediateCondition)
//	if _, err := i.PersistStatus(ctx, instance); err != nil {
//	    return i.Error(ctx, err, instance)
//	}
//	return i.Continue()
//
// # Anti-Patterns
//
// Ignoring PersistStatus errors:
//
//	// BAD: status update error is silently lost
//	_, _ = i.PersistStatus(ctx, instance)
//	return i.Requeue()
//
//	// GOOD: propagate the error
//	if _, err := i.PersistStatus(ctx, instance); err != nil {
//	    return i.Error(ctx, err, instance)
//	}
//	return i.Requeue()
//
// Ignoring the changed flag after a state transition:
//
//	// BAD: if status didn't change, no watch event fires,
//	// and reconciliation stops silently.
//	if _, err := i.PersistStatus(ctx, instance); err != nil {
//	    return i.Error(ctx, err, instance)
//	}
//	return i.Return()
//
//	// GOOD: use ReturnOnChange to handle flow control
//	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
//
// Missing CanHandle guard (causes infinite re-entry):
//
//	// BAD: CanHandle always returns true, Handle always sets the same condition,
//	// PersistStatus is a no-op, Return() stops the chain — but the watch event
//	// from a previous write may re-enter, creating a tight loop.
//	func (a myAction) CanHandle(_ context.Context, _ *rhtasv1.X) bool { return true }
//
//	// GOOD: gate on the state that this action transitions FROM
//	func (a myAction) CanHandle(_ context.Context, i *rhtasv1.X) bool {
//	    return meta.IsStatusConditionFalse(i.Status.Conditions, "Ready")
//	}
package action
