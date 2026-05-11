package actions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"time"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func NewExtractHSMRootAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &extractHSMRoot{}
}

type extractHSMRoot struct {
	action.BaseAction
}

func (e extractHSMRoot) Name() string {
	return "extract-hsm-root"
}

func (e extractHSMRoot) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	if instance.Spec.Certificate.CAType != rhtasv1alpha1.CATypePKCS11 {
		return false
	}
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	if c == nil {
		return false
	}
	return state.FromCondition(c) == state.Initialize
}

func (e extractHSMRoot) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	componentLabels := labels.ForComponent(ComponentName, instance.Name)

	pemData, err := e.resolveRootCert(ctx, instance)
	if err != nil {
		e.Logger.Info("Waiting for Fulcio root cert to become available", "error", err.Error())
		return e.Requeue()
	}

	existingMeta, _ := kubernetes.FindSecret(ctx, e.Client, instance.Namespace, FulcioCALabel)
	if existingMeta != nil {
		existingFull, err := kubernetes.GetSecret(e.Client, instance.Namespace, existingMeta.Name)
		if err == nil && bytes.Equal(existingFull.Data["cert"], pemData) {
			e.Logger.Info("HSM root CA secret already up-to-date", "secret", existingMeta.Name)
			return e.Continue()
		}
		e.Logger.Info("HSM root CA changed, rotating secret", "oldSecret", existingMeta.Name)
		if err := labels.Remove(ctx, existingMeta, e.Client, FulcioCALabel); err != nil {
			return e.Error(ctx, err, instance)
		}
		e.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "HSMRootCARotated", "LabelRemoved",
			"Removed '%s' label from old secret %s", FulcioCALabel, existingMeta.Name)
	}

	keyLabels := map[string]string{FulcioCALabel: "cert"}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("fulcio-pkcs11-cert-%s-", instance.Name),
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, e.Client,
		secret,
		ensure.ControllerReference[*v1.Secret](instance, e.Client),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(keyLabels)), keyLabels),
		kubernetes.EnsureSecretData(true, map[string][]byte{"cert": pemData}),
	); err != nil {
		return e.Error(ctx, fmt.Errorf("creating HSM root CA secret: %w", err), instance, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	instance.Status.Certificate.CARef = &rhtasv1alpha1.SecretKeySelector{
		Key: "cert",
		LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
			Name: secret.Name,
		},
	}

	e.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "HSMRootCAPublished", "Created",
		"HSM root CA published to secret %s", secret.Name)
	e.Logger.Info("HSM root CA secret created", "secret", secret.Name)

	return e.StatusUpdate(ctx, instance)
}

func (e extractHSMRoot) resolveRootCert(ctx context.Context, instance *rhtasv1alpha1.Fulcio) ([]byte, error) {
	baseURL := fmt.Sprintf("http://%s.%s.svc:%d", DeploymentName, instance.Namespace, ServerPort)

	inContainer, err := kubernetes.ContainerMode()
	if err == nil {
		if !inContainer && instance.Status.Url != "" {
			baseURL = instance.Status.Url
		}
	} else {
		klog.Info("Can't recognise operator mode - expecting in-container run")
	}

	return e.requestRootCert(ctx, baseURL)
}

func (e extractHSMRoot) requestRootCert(ctx context.Context, basePath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/rootCert", basePath), nil)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			e.Logger.V(1).Error(err, err.Error())
		}
	}(response.Body)

	if response.StatusCode == http.StatusOK {
		return io.ReadAll(response.Body)
	}
	return nil, fmt.Errorf("unexpected http response %s", response.Status)
}
