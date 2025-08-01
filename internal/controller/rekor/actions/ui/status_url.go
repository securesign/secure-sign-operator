package ui

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewStatusURLAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status-url"
}

func (i statusUrlAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return enabled(instance)
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var url string
	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v1.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("get ingress error: %w", err), instance)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		url = protocol + ingress.Spec.Rules[0].Host
	} else {
		url = fmt.Sprintf("http://%s.%s.svc", actions.SearchUiDeploymentName, instance.Namespace)
	}

	if url == instance.Status.RekorSearchUIUrl {
		return i.Continue()
	}

	instance.Status.RekorSearchUIUrl = url
	return i.StatusUpdate(ctx, instance)
}
