package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/fulcio/actions"
	v1 "k8s.io/api/core/v1"
)

func NewHandleFulcioCertAction() action.Action[v1alpha1.CTlog] {
	return &handleFulcioCert{}
}

type handleFulcioCert struct {
	action.BaseAction
}

func (g handleFulcioCert) Name() string {
	return "handle-fulcio-cert"
}

func (g handleFulcioCert) CanHandle(instance *v1alpha1.CTlog) bool {
	return instance.Status.Phase == v1alpha1.PhaseCreating && len(instance.Spec.RootCertificates) == 0
}

func (g handleFulcioCert) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {

	scr, err := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel)
	if err != nil {
		return g.Failed(err)
	}
	if scr == nil {
		//TODO: add status condition - waiting for fulcio
		return g.Requeue()
	}
	if scr.Data[scr.Labels[actions.FulcioCALabel]] == nil {
		return g.Failed(fmt.Errorf("can't find fulcio certificate in provided secret"))
	}

	instance.Spec.RootCertificates = append(instance.Spec.RootCertificates, v1alpha1.SecretKeySelector{
		LocalObjectReference: v1.LocalObjectReference{
			Name: scr.Name,
		},
		Key: scr.Labels[actions.FulcioCALabel],
	})
	return g.Update(ctx, instance)
}
