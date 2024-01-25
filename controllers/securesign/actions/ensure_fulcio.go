package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	actions2 "github.com/securesign/operator/controllers/fulcio/actions"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewFulcioAction() action.Action[rhtasv1alpha1.Securesign] {
	return &fulcioAction{}
}

type fulcioAction struct {
	action.BaseAction
}

func (i fulcioAction) Name() string {
	return "create fulcio"
}

func (i fulcioAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return instance.Status.Fulcio == ""
}

func (i fulcioAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	fulcio := &rhtasv1alpha1.Fulcio{}

	fulcio.Name = instance.Name
	fulcio.Namespace = instance.Namespace
	fulcio.Labels = constants.LabelsFor(actions2.ComponentName, fulcio.Name, instance.Name)
	fulcio.Spec = instance.Spec.Fulcio

	if err = controllerutil.SetControllerReference(instance, fulcio, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, fulcio); err != nil {
		return i.Failed(err)
	}

	if updated {
		instance.Status.Fulcio = fulcio.Name
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
