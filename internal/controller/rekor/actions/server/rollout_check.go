package server

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.Rekor] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Rekor]{
		Name:             "server rollout check",
		ConditionType:    actions.ServerCondition,
		DeploymentName:   actions.ServerDeploymentName,
		PromoteOnSuccess: true,
	})
}
