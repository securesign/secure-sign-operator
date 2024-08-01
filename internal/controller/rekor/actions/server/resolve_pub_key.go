package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RekorPubLabel       = constants.LabelNamespace + "/rekor.pub"
	pubSecretNameFormat = "rekor-public-%s-"
)

func NewResolvePubKeyAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &resolvePubKeyAction{}
}

type resolvePubKeyAction struct {
	action.BaseAction
}

func (i resolvePubKeyAction) Name() string {
	return "resolve public key"
}

func (i resolvePubKeyAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return meta.IsStatusConditionTrue(instance.Status.Conditions, actions.ServerCondition) &&
		instance.Status.PublicKeyRef == nil
}

func (i resolvePubKeyAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err       error
		publicKey []byte
	)

	// Resolve public key from Rekors API
	publicKey, err = i.resolvePubKey(*instance)
	if err != nil {
		errf := fmt.Errorf("ResolvePubKey: unable to resolve public key: %v", err)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: errf.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, errf, instance)
	}

	scrl := &metav1.PartialObjectMetadataList{}
	scrl.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})

	if err = k8sutils.FindByLabelSelector(ctx, i.Client, scrl, instance.Namespace, RekorPubLabel); err != nil {
		return i.Failed(fmt.Errorf("ResolvePubKey: find secrets failed: %w", err))
	}

	// Search if exists a secret with rhtas.redhat.com/rekor.pub label
	for _, secret := range scrl.Items {
		sks := rhtasv1alpha1.SecretKeySelector{
			LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: secret.Name},
			Key:                  secret.Labels[RekorPubLabel],
		}

		// Compare key from API and from discovered secret
		var sksPublicKey []byte
		sksPublicKey, err = k8sutils.GetSecretData(i.Client, instance.Namespace, &sks)
		if err != nil {
			return i.Failed(fmt.Errorf("ResolvePubKey: failed to read `%s` secret's data: %w", sks.Name, err))
		}

		if bytes.Equal(sksPublicKey, publicKey) {
			instance.Status.PublicKeyRef = &sks
			continue
		}

		// Remove label from secret
		if err = i.removeLabel(ctx, &secret); err != nil {
			return i.Failed(fmt.Errorf("ResolvePubKey: %w", err))
		}

		message := fmt.Sprintf("Removed '%s' label from %s secret", RekorPubLabel, secret.Name)
		i.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeySecretLabelRemoved", message)
		i.Logger.Info(message)
	}

	if instance.Status.PublicKeyRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	// Create new secret with public key
	const keyName = "public"
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
	labels[RekorPubLabel] = keyName

	newConfig := k8sutils.CreateImmutableSecret(
		fmt.Sprintf(pubSecretNameFormat, instance.Name),
		instance.Namespace,
		map[string][]byte{
			keyName: publicKey,
		},
		labels)

	if newConfig.Annotations == nil {
		newConfig.Annotations = make(map[string]string)
	}
	newConfig.Annotations[annotations.TreeId] = strconv.FormatInt(pointer.Int64Deref(instance.Status.TreeID, 0), 10)

	if err = i.Client.Create(ctx, newConfig); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	i.Recorder.Eventf(instance, v1.EventTypeNormal, "PublicKeySecretCreated", "New Rekor public key created: %s", newConfig.Name)
	c := meta.FindStatusCondition(instance.Status.Conditions, actions.ServerCondition)
	c.Message = "Public key resolved"
	meta.SetStatusCondition(&instance.Status.Conditions, *c)
	return i.StatusUpdate(ctx, instance)
}

func (i resolvePubKeyAction) resolvePubKey(instance rhtasv1alpha1.Rekor) ([]byte, error) {
	var (
		data []byte
		err  error
	)

	if data, err = i.requestPublicKey(fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, instance.Namespace)); err == nil {
		return data, nil
	}
	i.Logger.Info("retrying to get rekor public key")

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

func (i resolvePubKeyAction) removeLabel(ctx context.Context, object *metav1.PartialObjectMetadata) error {
	object.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	patch, err := json.Marshal([]map[string]string{
		{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/labels/%s", strings.ReplaceAll(RekorPubLabel, "/", "~1")),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	err = i.Client.Patch(ctx, object, client.RawPatch(types.JSONPatchType, patch))
	if err != nil {
		return fmt.Errorf("unable to remove '%s' label from secret: %w", RekorPubLabel, err)
	}

	return nil
}
