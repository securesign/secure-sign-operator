package trillian

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/trillian/utils"
)

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "create"
}

func (i initializeAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	//log := ctrllog.FromContext(ctx)
	tree, err := utils.CreateTrillianTree(ctx, instance.Status.Url)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Trillian tree: %w", err)
	}

	instance.Status.TreeID = tree.TreeId
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
