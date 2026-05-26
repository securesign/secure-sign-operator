package ui

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	v12 "k8s.io/api/networking/v1"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/console/actions"
	"k8s.io/apimachinery/pkg/types"
)

func NewStatusUrlAction() action.Action[*rhtasv1alpha1.Console] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status url"
}

func (i statusUrlAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {
	var url string
	var externalAccessEnabled bool

	if instance.Spec.UI.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Error(ctx, err, instance)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		url = protocol + ingress.Spec.Rules[0].Host
		externalAccessEnabled = true
	} else {
		url = fmt.Sprintf("http://%s.%s.svc", actions.UIDeploymentName, instance.Namespace)
		externalAccessEnabled = false
	}

	if url == instance.Status.UI.Url && instance.Status.UI.ExternalAccess.Enabled == externalAccessEnabled {
		return i.Continue()
	}

	instance.Status.UI.Url = url
	instance.Status.UI.ExternalAccess.Enabled = externalAccessEnabled
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
