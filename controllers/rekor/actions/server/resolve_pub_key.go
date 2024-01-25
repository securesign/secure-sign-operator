package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const pubSecretNameFormat = "rekor-%s-public-key"

func NewResolvePubKeyAction() action.Action[rhtasv1alpha1.Rekor] {
	return &resolvePubKeyAction{}
}

type resolvePubKeyAction struct {
	action.BaseAction
}

func (i resolvePubKeyAction) Name() string {
	return "resolve public key"
}

func (i resolvePubKeyAction) CanHandle(instance *rhtasv1alpha1.Rekor) bool {
	return instance.Status.Phase != rhtasv1alpha1.PhaseInitialize
}

func (i resolvePubKeyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)
	secrets, err := i.findSecret(ctx, instance.Namespace)
	if err != nil {
		return i.Failed(err)
	}
	if len(secrets.Items) > 0 {
		return i.Continue()
	}

	key, err := i.resolvePubKey(*instance)
	if err != nil {
		return i.Failed(err)
	}

	secretName := fmt.Sprintf(pubSecretNameFormat, instance.Name)
	labels := constants.LabelsFor(actions.ServerComponentName, secretName, instance.Name)
	labels[RekorPubLabel] = "public"

	secret := k8sutils.CreateSecret(secretName, instance.Namespace,
		map[string][]byte{
			"public": key,
		}, labels)
	if err = controllerutil.SetControllerReference(instance, secret, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if _, err = i.Ensure(ctx, secret); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create secret: %w", err), instance)
	}
	return i.Continue()
}

func (i resolvePubKeyAction) findSecret(ctx context.Context, namespace string) (*v1.SecretList, error) {
	list := &v1.SecretList{}
	err := i.Client.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{RekorPubLabel: "public"})
	return list, err
}

func (i resolvePubKeyAction) resolvePubKey(instance rhtasv1alpha1.Rekor) ([]byte, error) {
	var (
		pubKeyResponse *http.Response
		err            error
	)
	for retry := 1; retry < 5; retry++ {
		time.Sleep(time.Duration(retry) * time.Second)
		pubKeyResponse, err = http.Get(fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace) + "/api/v1/log/publicKey")
		if err == nil && pubKeyResponse.StatusCode == http.StatusOK {
			continue
		}
		i.Logger.Info("retrying to get rekor public key")
	}

	if err != nil || pubKeyResponse.StatusCode != http.StatusOK {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return nil, err
	}
	return io.ReadAll(pubKeyResponse.Body)

}
