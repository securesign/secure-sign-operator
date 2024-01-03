package tuf

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils"
	tufutils "github.com/securesign/operator/controllers/tuf/utils"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const tufDeploymentName = "tuf"

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseInitialization
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	//log := ctrllog.FromContext(ctx)

	var err error

	// TODO: migrate code to the operator
	copyJob := tufutils.InitTufCopyJob(instance.Namespace, "tuf-secret-copy-job")
	if err = i.Client.Create(ctx, copyJob); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create copy job: %w", err)
	}

	db := tufutils.CreateTufDeployment(instance.Namespace, instance.Spec.Image, tufDeploymentName)
	controllerutil.SetControllerReference(instance, db, i.Client.Scheme())
	if err = i.Client.Create(ctx, db); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create TUF: %w", err)
	}

	tuf := utils.CreateService(instance.Namespace, "tuf", "tuf", "tuf", 8080)
	//patch the pregenerated service
	tuf.Spec.Ports[0].Port = 80
	controllerutil.SetControllerReference(instance, tuf, i.Client.Scheme())
	if err = i.Client.Create(ctx, tuf); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}
	instance.Status.Phase = rhtasv1alpha1.PhaseInitialization
	return instance, nil
}
