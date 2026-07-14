package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/constants"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.CTlog] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.CTlog]{
		Name:           "rollout check",
		ConditionType:  constants.ReadyCondition,
		DeploymentName: DeploymentName,
	})
}
