package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/controller/rekor/actions"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.Rekor] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Rekor]{
		Name:             "redis rollout check",
		ConditionType:    actions.RedisCondition,
		DeploymentName:   actions.RedisDeploymentName,
		Enabled:          enabled,
		PromoteOnSuccess: true,
	})
}
