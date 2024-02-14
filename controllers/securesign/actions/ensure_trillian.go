package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewTrillianAction() action.Action[rhtasv1alpha1.Securesign] {
	return &trillianAction{}
}

type trillianAction struct {
	action.BaseAction
}

func (i trillianAction) Name() string {
	return "create trillian"
}

func (i trillianAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return instance.Status.Trillian == ""
}

func (i trillianAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	trillian := &rhtasv1alpha1.Trillian{}

	trillian.Name = instance.Name
	trillian.Namespace = instance.Namespace
	trillian.Labels = constants.LabelsFor("trillian", trillian.Name, instance.Name)
	trillian.Spec = instance.Spec.Trillian

	if err = controllerutil.SetControllerReference(instance, trillian, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, trillian); err != nil {
		return i.Failed(err)
	}

	if updated {
		instance.Status.Trillian = trillian.Name
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
