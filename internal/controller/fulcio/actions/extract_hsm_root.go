package actions

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	fulcioLabels "github.com/securesign/operator/internal/labels"
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

func (e extractHSMRoot) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	if instance.Spec.Certificate.CAType != rhtasv1alpha1.CATypePKCS11 {
		return false
	}
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	if c == nil {
		return false
	}
	if state.FromCondition(c) != state.Initialize {
		return false
	}

	existing, _ := kubernetes.FindSecret(context.Background(), e.Client, instance.Namespace, FulcioCALabel)
	return existing == nil
}

func (e extractHSMRoot) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	componentLabels := fulcioLabels.ForComponent(ComponentName, instance.Name)

	pemData, err := e.resolveRootCert(instance)
	if err != nil {
		e.Logger.Info("Waiting for Fulcio root cert to become available", "error", err.Error())
		return e.Requeue()
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

	e.Recorder.Event(instance, v1.EventTypeNormal, "HSMRootCAPublished",
		fmt.Sprintf("HSM root CA published to secret %s", secret.Name))
	e.Logger.Info("HSM root CA secret created", "secret", secret.Name)

	return e.Continue()
}

func (e extractHSMRoot) resolveRootCert(instance *rhtasv1alpha1.Fulcio) ([]byte, error) {
	url := fmt.Sprintf("http://%s.%s.svc:%d", DeploymentName, instance.Namespace, ServerPort)

	inContainer, err := kubernetes.ContainerMode()
	if err == nil {
		if !inContainer && instance.Status.Url != "" {
			url = instance.Status.Url
		}
	} else {
		klog.Info("Can't recognise operator mode - expecting in-container run")
	}

	return e.requestRootCert(url)
}

func (e extractHSMRoot) requestRootCert(basePath string) ([]byte, error) {
	response, err := http.Get(fmt.Sprintf("%s/api/v1/rootCert", basePath))
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
