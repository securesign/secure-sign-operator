package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.Tuf] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Tuf]{
		Name:           "rollout check",
		ConditionType:  constants.ReadyCondition,
		DeploymentName: tufConstants.DeploymentName,
	})
}
