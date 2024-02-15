package db

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	actions2 "github.com/securesign/operator/controllers/trillian/actions"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(instance *rhtasv1alpha1.Trillian) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating && instance.Spec.Db.Create
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err       error
		updated   bool
		openshift bool
	)

	openshift = kubernetes.IsOpenShift(i.Client)

	labels := constants.LabelsFor(actions2.ComponentName, actions2.DbDeploymentName, instance.Name)
	db := trillianUtils.CreateTrillDb(instance.Namespace, constants.TrillianDbImage,
		actions2.DbDeploymentName,
		actions2.RBACName,
		instance.Spec.Db.PvcName,
		*instance.Spec.Db.DatabaseSecretRef,
		openshift,
		labels)
	if err = controllerutil.SetControllerReference(instance, db, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for DB Deployment: %w", err))
	}

	if updated, err = i.Ensure(ctx, db); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian DB: %w", err), instance)
	}

	if updated {
		return i.Return()
	} else {
		return i.Continue()
	}

}
