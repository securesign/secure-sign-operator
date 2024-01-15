package rekor

import (
	"context"
	"fmt"
	"github.com/securesign/operator/controllers/common/action"
	"io"
	"net/http"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewWaitAction() action.Action[rhtasv1alpha1.Rekor] {
	return &waitAction{}
}

type waitAction struct {
	action.BaseAction
}

func (i waitAction) Name() string {
	return "wait"
}

func (i waitAction) CanHandle(Rekor *rhtasv1alpha1.Rekor) bool {
	return Rekor.Status.Phase == rhtasv1alpha1.PhaseInitialize
}

func (i waitAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
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

	var pubKeyResponse *http.Response
	for retry := 1; retry < 5; retry++ {
		time.Sleep(time.Duration(retry) * time.Second)
		pubKeyResponse, err = http.Get(instance.Status.Url + "/api/v1/log/publicKey")
		if err == nil && pubKeyResponse.StatusCode == http.StatusOK {
			continue
		}
		i.Logger.Info("retrying to get rekor public key")
	}

	if err != nil || pubKeyResponse.StatusCode != http.StatusOK {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	body, err := io.ReadAll(pubKeyResponse.Body)
	if err != nil || pubKeyResponse.StatusCode != http.StatusOK {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, err
	}
	secret := commonUtils.CreateSecret("rekor-public-key", instance.Namespace, map[string][]byte{"key": body}, labels)
	controllerutil.SetControllerReference(instance, secret, i.Client.Scheme())
	if err = i.Client.Create(ctx, secret); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create rekor-public-key secret: %w", err)
	}
	instance.Status.Phase = rhtasv1alpha1.PhaseReady
	return instance, nil
}
