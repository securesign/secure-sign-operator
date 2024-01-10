package trillian

import (
	"context"
	"fmt"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/trillian/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "create"
}

func (i initializeAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	var caCert []byte = nil
	if instance.Spec.External {
		scr := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: instance.Namespace,
			},
			Data: map[string]string{},
		}
		scr.Annotations = map[string]string{"service.beta.openshift.io/inject-cabundle": "true"}
		if err := i.Client.Create(ctx, scr); err != nil {
			return instance, err
		}
		// wait some time for the certificate injection
		time.Sleep(time.Second)

		if err := i.Client.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      scr.Name,
		}, scr); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not get trillian CaCert: %w", err)
		}
		caCert = []byte(scr.Data["service-ca.crt"])
	}
	tree, err := utils.CreateTrillianTree(ctx, instance.Status.Url, caCert)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Trillian tree: %w", err)
	}

	instance.Status.TreeID = tree.TreeId
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
