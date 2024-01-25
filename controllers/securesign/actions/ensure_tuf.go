package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	actions2 "github.com/securesign/operator/controllers/tuf/actions"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewTufAction() action.Action[rhtasv1alpha1.Securesign] {
	return &tufAction{}
}

type tufAction struct {
	action.BaseAction
}

func (i tufAction) Name() string {
	return "create tuf"
}

func (i tufAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return instance.Status.Tuf == ""
}

func (i tufAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	tuf := &rhtasv1alpha1.Tuf{}

	tuf.Name = instance.Name
	tuf.Namespace = instance.Namespace
	tuf.Labels = constants.LabelsFor(actions2.ComponentName, tuf.Name, instance.Name)
	tuf.Spec = instance.Spec.Tuf

	if err = controllerutil.SetControllerReference(instance, tuf, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, tuf); err != nil {
		return i.Failed(err)
	}

	if updated {
		instance.Status.Tuf = tuf.Name
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
