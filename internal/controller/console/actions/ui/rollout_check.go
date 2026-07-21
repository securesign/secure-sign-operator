package ui

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/controller/console/actions"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.Console] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Console]{
		Name:           "ui rollout check",
		ConditionType:  actions.UICondition,
		DeploymentName: actions.UIDeploymentName,
	})
}
