package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const pubSecretNameFormat = "rekor-public-%s-"

func NewResolvePubKeyAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &resolvePubKeyAction{}
}

type resolvePubKeyAction struct {
	action.BaseAction
}

func (i resolvePubKeyAction) Name() string {
	return "resolve public key"
}

func (i resolvePubKeyAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if (c.Reason != constants.Initialize && c.Reason != constants.Ready) || !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.ServerCondition) {
		return false
	}

	if scr, err := k8sutils.GetSecret(i.Client, instance.Namespace, instance.Status.Signer.KeyRef.Name); err == nil {
		if _, ok := scr.Labels[RekorPubLabel]; ok {
			return false
		}
	}

	if scr, _ := k8sutils.FindSecret(ctx, i.Client, instance.Namespace, RekorPubLabel); scr != nil {
		if expected, err := i.resolvePubKey(*instance); err == nil {
			return !bytes.Equal(scr.Data[scr.Labels[RekorPubLabel]], expected)
		}
	} else {
		return true
	}
	return false
}

func (i resolvePubKeyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)

	key, err := i.resolvePubKey(*instance)
	if err != nil {
		return i.Failed(err)
	}

	keyName := "public"
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	labels[RekorPubLabel] = keyName

	newConfig := k8sutils.CreateImmutableSecret(fmt.Sprintf(pubSecretNameFormat, instance.Name), instance.Namespace,
		map[string][]byte{
			keyName: key,
		}, labels)
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}

	// ensure that only new key is exposed
	if err = i.Client.DeleteAllOf(ctx, &v1.Secret{}, client.InNamespace(instance.Namespace), client.MatchingLabels(constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)), client.HasLabels{RekorPubLabel}); err != nil {
		return i.Failed(err)
	}

	if err = i.Client.Create(ctx, newConfig); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	i.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeySecretCreated", "New Rekor public key created: "+newConfig.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Server config created"})
	return i.StatusUpdate(ctx, instance)
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
		return nil, err
	}
	return io.ReadAll(pubKeyResponse.Body)

}
