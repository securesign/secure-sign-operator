/*
Package deploymentRollout implements a generic action that keeps a status
condition in sync with the live rollout status of a component's Deployment.

CanHandle gates on the CR's overall Ready condition reaching state.Initialize
or later — the phase at which the component's Deployment is expected to
exist. It never gates on ConditionType itself, so this action keeps
re-verifying the rollout on every reconcile even once the CR is Ready,
catching a later regression (image bump, deleted Deployment, stuck rollout)
that would otherwise go undetected forever.

Handle checks commonUtils.DeploymentIsRunningByName:

  - Not rolled out: set ConditionType to False/Initialize with the underlying
    error as Message, and requeue after 5s. If ConditionType is a component's
    own sub-condition (not the CR's overall Ready condition), the Ready
    condition is also demoted to False/Initialize with a "Waiting for
    <deployment>" message — otherwise a sub-component regression would be
    invisible at the top level, since the chain halts here (RequeueAfter)
    before ever reaching transitions.NewToReadyPhaseAction, and a stale
    True Ready would never get corrected.
  - Rolled out, PromoteOnSuccess=false: Continue(), leaving promotion to
    transitions.NewToReadyPhaseAction (components tracking the overall Ready
    condition directly).
  - Rolled out, PromoteOnSuccess=true: set ConditionType to True/Ready and
    persist (components tracking their own sub-condition, e.g. Rekor's
    ServerCondition or Trillian's DbCondition).

Usage:

	deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Rekor]{
	    Name:             "rollout check",
	    ConditionType:    actions.ServerCondition,
	    DeploymentName:   actions.ServerDeploymentName,
	    PromoteOnSuccess: true,
	})
*/
package deploymentRollout
