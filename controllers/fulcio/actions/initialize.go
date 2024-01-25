package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/constants"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(instance *rhtasv1alpha1.Fulcio) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := constants.LabelsForComponent(ComponentName, instance.Name)
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		// deployment is watched - no need to requeue
		return i.Return()
	}

	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err = i.Client.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Failed(err)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		instance.Status.Url = protocol + ingress.Spec.Rules[0].Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", DeploymentName, instance.Namespace)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseReady)})

	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return i.StatusUpdate(ctx, instance)
}
