package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/fulcio/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating && len(instance.Spec.RootCertificates) == 0
}

func (g handleFulcioCert) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {

	scr, err := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel)
	if err != nil {
		return g.Failed(err)
	}
	if scr == nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   CertCondition,
			Status: metav1.ConditionFalse,
			Reason: "Cert not found",
		})
		return g.StatusUpdate(ctx, instance)
	} else {
		if !meta.IsStatusConditionTrue(instance.Status.Conditions, CertCondition) {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   CertCondition,
				Status: metav1.ConditionTrue,
				Reason: "Resolved",
			},
			)
			return g.StatusUpdate(ctx, instance)
		}
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
