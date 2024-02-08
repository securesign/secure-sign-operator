package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewRekorAction() action.Action[rhtasv1alpha1.Securesign] {
	return &rekorAction{}
}

type rekorAction struct {
	action.BaseAction
}

func (i rekorAction) Name() string {
	return "create rekor"
}

func (i rekorAction) CanHandle(instance *rhtasv1alpha1.Securesign) bool {
	return instance.Status.Rekor == ""
}

func (i rekorAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		updated bool
	)
	rekor := &rhtasv1alpha1.Rekor{}

	rekor.Name = instance.Name
	rekor.Namespace = instance.Namespace
	rekor.Labels = constants.LabelsFor("rekor", rekor.Name, instance.Name)
	rekor.Spec = instance.Spec.Rekor

	if err = controllerutil.SetControllerReference(instance, rekor, i.Client.Scheme()); err != nil {
		return i.Failed(err)
	}

	if updated, err = i.Ensure(ctx, rekor); err != nil {
		return i.Failed(err)
	}

	if updated {
		instance.Status.Rekor = rekor.Name
		return i.StatusUpdate(ctx, instance)
	}

	return i.Continue()
}
