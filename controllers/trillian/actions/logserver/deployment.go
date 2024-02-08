package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const serverPort = 8091

func NewDeployAction() action.Action[rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(actions2.ComponentName, actions2.LogserverDeploymentName, instance.Name)
	server := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianServerImage,
		actions2.LogserverDeploymentName,
		actions2.RBACName,
		*instance.Spec.Db.DatabaseSecretRef,
		labels)
	server.Spec.Template.Spec.Containers[0].Ports = append(server.Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{
		Protocol:      corev1.ProtocolTCP,
		ContainerPort: 8090,
	})

	if err = controllerutil.SetControllerReference(instance, server, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for server: %w", err))
	}

	if updated, err = i.Ensure(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Trillian server: %w", err), instance)
	}

	if updated {
		return i.Return()
	} else {
		return i.Continue()
	}
}
