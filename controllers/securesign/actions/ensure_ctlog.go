package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	actions2 "github.com/securesign/operator/controllers/ctlog/actions"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCtlogAction() action.Action[rhtasv1alpha1.Securesign] {
	return &ctlogAction{}
}

type ctlogAction struct {
	action.BaseAction
}

func (i ctlogAction) Name() string {
	return "create ctlog"
}

func (i ctlogAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return instance.Status.CTlog == ""
}

func (i ctlogAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	ctlog := &rhtasv1alpha1.CTlog{}

	ctlog.Name = instance.Name
	ctlog.Namespace = instance.Namespace
	ctlog.Labels = constants.LabelsFor(actions2.ComponentName, ctlog.Name, instance.Name)
	ctlog.Spec = instance.Spec.Ctlog

	if err = controllerutil.SetControllerReference(instance, ctlog, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, ctlog); err != nil {
		return i.Failed(err)
	}

	if updated {
		instance.Status.CTlog = ctlog.Name
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
