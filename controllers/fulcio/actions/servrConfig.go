package actions

import (
	"context"
	"encoding/json"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cmName = "fulcio-server-config"
)

func NewServerConfigAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

func (i serverConfig) CanHandle(instance *rhtasv1alpha1.Fulcio) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating || instance.Status.Phase == rhtasv1alpha1.PhaseReady
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	if instance.Status.Phase != rhtasv1alpha1.PhaseCreating {
		instance.Status.Phase = rhtasv1alpha1.PhaseCreating
		return i.Update(ctx, instance)
	}

	var (
		err     error
		updated bool
	)
	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	config, err := json.Marshal(instance.Spec.Config)
	if err != nil {
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	cm := kubernetes.InitConfigmap(instance.Namespace, cmName, labels, map[string]string{
		"config.json": string(config),
	})
	if err = controllerutil.SetControllerReference(instance, cm, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
	}
	if updated, err = i.Ensure(ctx, cm); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if updated {
		return i.Requeue()
	} else {
		return i.Continue()
	}

}
