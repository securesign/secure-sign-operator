package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[rhtasv1alpha1.Trillian] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseCreating || trillian.Status.Phase == rhtasv1alpha1.PhaseReady
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {

	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions2.ComponentName, actions2.LogserverDeploymentName, instance.Name)
	logserverService := k8sutils.CreateService(instance.Namespace, actions2.LogserverDeploymentName, serverPort, labels)

	if err = controllerutil.SetControllerReference(instance, logserverService, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for logserver Service: %w", err))
	}

	if updated, err = i.Ensure(ctx, logserverService); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create logserver Service: %w", err), instance)
	}

	if updated {
		return i.Return()
	} else {
		return i.Continue()
	}

}
