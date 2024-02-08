package server

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/rekor/actions"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewInitializeUrlAction() action.Action[rhtasv1alpha1.Rekor] {
	return &initializeUrlAction{}
}

type initializeUrlAction struct {
	action.BaseAction
}

func (i initializeUrlAction) Name() string {
	return "initialize url"
}

func (i initializeUrlAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseInitialize && !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.ServerComponentName+"Ready")
}

func (i initializeUrlAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)

	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err = i.Client.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Failed(err)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		instance.Status.Url = protocol + ingress.Spec.Rules[0].Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.ServerComponentName + "Ready",
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseReady)})

	return i.StatusUpdate(ctx, instance)
}
