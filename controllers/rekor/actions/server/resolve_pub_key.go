package server

import (
	"bytes"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

const pubSecretNameFormat = "rekor-public-%s-"

func NewResolvePubKeyAction() action.Action[rhtasv1alpha1.Rekor] {
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

	return instance.Status.PublicKeyRef == nil
}

func (i resolvePubKeyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
		scr *metav1.PartialObjectMetadata
		sks *rhtasv1alpha1.SecretKeySelector
		publicKey []byte
	)

	if scr, _ = k8sutils.FindSecret(ctx, i.Client, instance.Namespace, RekorPubLabel); scr != nil {
		if key, ok := scr.GetLabels()[RekorPubLabel]; ok {
			sks = &rhtasv1alpha1.SecretKeySelector{
				LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: scr.Name},
				Key: key,
			}
		}
	}

	// Use discovered public key if it is stored in same secret like private key
	if sks != nil && instance.Status.Signer.KeyRef != nil && sks.Name == instance.Status.Signer.KeyRef.Name {
		instance.Status.PublicKeyRef = sks
		return i.StatusUpdate(ctx, instance)
	}

	// Resolve public key from Rekors API
	publicKey, err = i.resolvePubKey(*instance)
	if err != nil {
		return i.Failed(err)
	}

	// Search if exists a secret with rhtas.redhat.com/rekor.pub label

	// Compare key from API and from discovered secret
	if sks != nil && scr != nil {
		var sksPublicKey []byte
		sksPublicKey, err = k8sutils.GetSecretData(i.Client, instance.Namespace, sks)
		if err != nil {
			return i.Failed(err)
		}

		if bytes.Equal(sksPublicKey, publicKey) {
			instance.Status.PublicKeyRef = sks
			return i.StatusUpdate(ctx, instance)
		}

		// Delete secret
		if err = i.Client.Delete(ctx, scr); err != nil {
			return i.Failed(err)
		}
		i.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeySecretDeleted", "Secret with public key deleted: " + scr.Name)
	}

	// Create new secret with public key
	keyName := "public"
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
	labels[RekorPubLabel] = keyName

	newConfig := k8sutils.CreateImmutableSecret(fmt.Sprintf(pubSecretNameFormat, instance.Name), instance.Namespace,
		map[string][]byte{
			keyName: publicKey,
		}, labels)
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
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
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: actions.ServerCondition,
		Status: metav1.ConditionFalse,
		Reason: constants.Ready,
		Message: "Server config created",
	})
	return i.StatusUpdate(ctx, instance)
}

func (i resolvePubKeyAction) resolvePubKey(instance rhtasv1alpha1.Rekor) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	for retry := 1; retry < 5; retry++ {
		time.Sleep(time.Duration(retry) * time.Second)
		if data, err = i.requestPublicKey(fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace)); err == nil{
			return data, nil
		}
		i.Logger.Info("retrying to get rekor public key")
	}

	return nil, err
}

func (i resolvePubKeyAction) requestPublicKey(basePath string) ([]byte, error) {
	response, err := http.Get(fmt.Sprintf("%s/api/v1/log/publicKey", basePath))
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			i.Logger.V(1).Error(err, err.Error())
		}
	}(response.Body)

	if response.StatusCode == http.StatusOK {
		return io.ReadAll(response.Body)
	}
	return nil, fmt.Errorf("unexpected http response %s", response.Status)
}
