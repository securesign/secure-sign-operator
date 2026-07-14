package db

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/deploymentRollout"
	"github.com/securesign/operator/internal/controller/trillian/actions"
)

func NewRolloutCheckAction() action.Action[*rhtasv1.Trillian] {
	return deploymentRollout.NewAction(deploymentRollout.Config[*rhtasv1.Trillian]{
		Name:             "db rollout check",
		ConditionType:    actions.DbCondition,
		DeploymentName:   actions.DbDeploymentName,
		Enabled:          enabled,
		PromoteOnSuccess: true,
	})
}
