package tuf

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/securesign/operator/controllers/common/action"
	v12 "k8s.io/api/networking/v1"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
)

func NewWaitAction() action.Action[rhtasv1alpha1.Tuf] {
	return &waitAction{}
}

type waitAction struct {
	action.BaseAction
}

func (i waitAction) Name() string {
	return "wait"
}

func (i waitAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	var (
		ok  bool
		err error
	)
	labels := commonUtils.FilterCommonLabels(instance.Labels)
	labels[commonUtils.ComponentLabel] = ComponentName
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	if !ok {
		return instance, nil
	}

	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingressList := &v12.IngressList{}
		err = i.Client.List(ctx, ingressList, client2.InNamespace(instance.Namespace), client2.MatchingLabels(labels), client2.Limit(1))
		if err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, err
		}
		if len(ingressList.Items) != 1 {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, errors.New("can't find ingress object")
		}
		if len(ingressList.Items[0].Spec.TLS) > 0 {
			protocol = "https://"
		}
		instance.Status.Url = protocol + ingressList.Items[0].Spec.Rules[0].Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", DeploymentName, instance.Namespace)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseReady)})

	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
