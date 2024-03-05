package actions

import (
	"context"
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/fulcio/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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

func (g handleFulcioCert) CanHandle(ctx context.Context, instance *v1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c.Reason != constants.Creating && c.Reason != constants.Ready {
		return false
	}

	if len(instance.Status.RootCertificates) == 0 {
		return true
	}

	if !equality.Semantic.DeepDerivative(instance.Spec.RootCertificates, instance.Status.RootCertificates) {
		return true
	}

	if scr, _ := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel); scr != nil {
		return !slices.Contains(instance.Status.RootCertificates, v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{Name: scr.Name},
			Key:                  scr.Labels[actions.FulcioCALabel],
		})
	}
	return false
}

func (g handleFulcioCert) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {

	scr, err := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, actions.FulcioCALabel)
	if err != nil {
		return g.Failed(err)
	}
	if scr == nil && len(instance.Spec.RootCertificates) == 0 {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: "Cert not found",
		})
		g.StatusUpdate(ctx, instance)
		return g.Requeue()
	}

	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Creating {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Creating,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}

	instance.Status.RootCertificates = append(instance.Spec.RootCertificates, v1alpha1.SecretKeySelector{
		LocalObjectReference: v1alpha1.LocalObjectReference{
			Name: scr.Name,
		},
		Key: scr.Labels[actions.FulcioCALabel],
	})

	// invalidate server config
	if instance.Status.ServerConfigRef != nil {
		if err = g.Client.Delete(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Status.ServerConfigRef.Name,
				Namespace: instance.Namespace,
			},
		}); err != nil {
			return g.Failed(err)
		}
		instance.Status.ServerConfigRef = nil
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	},
	)
	return g.StatusUpdate(ctx, instance)
}
