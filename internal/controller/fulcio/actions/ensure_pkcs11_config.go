package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1beta1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch

const (
	PKCS11ConfigCondition = "PKCS11ConfigAvailable"

	pkcs11CredSecretFormat = "fulcio-pkcs11-creds-%s-"
	pkcs11ConfSecretFormat = "fulcio-pkcs11-conf-%s-"
	pkcs11VolumesCMFormat  = "fulcio-pkcs11-%s-%s-"

	PKCS11CredLabel      = labels.LabelNamespace + "/fulcio.pkcs11.credentials"
	PKCS11ConfLabel      = labels.LabelNamespace + "/fulcio.pkcs11.config"
	PKCS11VolLabelPrefix = labels.LabelNamespace + "/fulcio.pkcs11.volume."
)

type crypto11Conf struct {
	Path       string `json:"Path"`
	TokenLabel string `json:"TokenLabel"`
	Pin        string `json:"Pin"`
}

func NewEnsurePKCS11ConfigAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &ensurePKCS11Config{}
}

type ensurePKCS11Config struct {
	action.BaseAction
}

func (e ensurePKCS11Config) Name() string {
	return "ensure-pkcs11-config"
}

func (e ensurePKCS11Config) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	if instance.Spec.Certificate.CAType != rhtasv1alpha1.CATypePKCS11 {
		return false
	}
	if state.FromInstance(instance, constants.ReadyCondition) < state.Creating {
		return false
	}
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11ConfigCondition) {
		return true
	}
	return e.hasPKCS11ConfigDrift(instance)
}

func (e ensurePKCS11Config) hasPKCS11ConfigDrift(instance *rhtasv1alpha1.Fulcio) bool {
	if instance.Status.Certificate == nil || instance.Status.Certificate.PKCS11 == nil {
		return true
	}
	spec := instance.Spec.Certificate.PKCS11
	status := instance.Status.Certificate.PKCS11

	if !equality.Semantic.DeepDerivative(spec.CredentialsRef, status.CredentialsRef) {
		return true
	}
	if !equality.Semantic.DeepDerivative(spec.PKCS11ConfigRef, status.PKCS11ConfigRef) {
		return true
	}
	if !equality.Semantic.DeepDerivative(spec.KeyConfig, status.KeyConfig) {
		return true
	}
	return false
}

func (e ensurePKCS11Config) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	if meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11ConfigCondition) {
		return e.handleRotation(ctx, instance)
	}

	p := instance.Spec.Certificate.PKCS11
	if p == nil {
		return e.Error(ctx, fmt.Errorf("pkcs11 config is nil"), instance)
	}

	componentLabels := labels.For(ComponentName, DeploymentName, instance.Name)

	if instance.Status.Certificate == nil {
		instance.Status.Certificate = instance.Spec.Certificate.DeepCopy()
	}
	statusPKCS11 := instance.Status.Certificate.PKCS11

	if err := e.ensureCredentials(ctx, instance, p, statusPKCS11, componentLabels); err != nil {
		return e.Error(ctx, err, instance, metav1.Condition{
			Type:    PKCS11ConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if err := e.ensurePKCS11Conf(ctx, instance, p, statusPKCS11, componentLabels); err != nil {
		return e.Error(ctx, err, instance, metav1.Condition{
			Type:    PKCS11ConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if err := e.ensureInlineVolumes(ctx, instance, p, statusPKCS11, componentLabels); err != nil {
		return e.Error(ctx, err, instance, metav1.Condition{
			Type:    PKCS11ConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   PKCS11ConfigCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	return e.StatusUpdate(ctx, instance)
}

func (e ensurePKCS11Config) ensureCredentials(
	ctx context.Context,
	instance *rhtasv1alpha1.Fulcio,
	spec *rhtasv1alpha1.PKCS11Config,
	status *rhtasv1alpha1.PKCS11Config,
	componentLabels map[string]string,
) error {
	if spec.CredentialsRef != nil {
		status.CredentialsRef = spec.CredentialsRef
		return nil
	}

	if spec.Pin == "" {
		return fmt.Errorf("either credentialsRef or pin must be specified")
	}

	existing, err := kubernetes.FindSecret(ctx, e.Client, instance.Namespace, PKCS11CredLabel)
	if err == nil && existing != nil {
		status.CredentialsRef = &rhtasv1alpha1.SecretKeySelector{
			Key:                  "pin",
			LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: existing.Name},
		}
		return nil
	}

	keyLabels := map[string]string{PKCS11CredLabel: "pin"}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(pkcs11CredSecretFormat, instance.Name),
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, e.Client,
		secret,
		ensure.ControllerReference[*v1.Secret](instance, e.Client),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(keyLabels)), keyLabels),
		kubernetes.EnsureSecretData(true, map[string][]byte{"pin": []byte(spec.Pin)}),
	); err != nil {
		return fmt.Errorf("creating PIN secret: %w", err)
	}

	e.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "PKCS11CredentialsCreated", "Created", "PIN secret created: %s", secret.Name)
	status.CredentialsRef = &rhtasv1alpha1.SecretKeySelector{
		Key:                  "pin",
		LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: secret.Name},
	}
	return nil
}

func (e ensurePKCS11Config) ensurePKCS11Conf(
	ctx context.Context,
	instance *rhtasv1alpha1.Fulcio,
	spec *rhtasv1alpha1.PKCS11Config,
	status *rhtasv1alpha1.PKCS11Config,
	componentLabels map[string]string,
) error {
	if spec.PKCS11ConfigRef != nil {
		status.PKCS11ConfigRef = spec.PKCS11ConfigRef
		return nil
	}

	if spec.TokenLabel == "" || spec.LibraryPath == "" {
		return fmt.Errorf("either pkcs11ConfigRef or (tokenLabel + libraryPath) must be specified")
	}

	existing, err := kubernetes.FindSecret(ctx, e.Client, instance.Namespace, PKCS11ConfLabel)
	if err == nil && existing != nil {
		status.PKCS11ConfigRef = &rhtasv1alpha1.SecretKeySelector{
			Key:                  "crypto11.conf",
			LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: existing.Name},
		}
		return nil
	}

	pin := spec.Pin
	if status.CredentialsRef != nil {
		pinData, pinErr := kubernetes.GetSecretData(e.Client, instance.Namespace, toAlphaSecretKeySelector(status.CredentialsRef))
		if pinErr == nil {
			pin = string(pinData)
		}
	}

	conf := crypto11Conf{
		Path:       fmt.Sprintf("%s/%s", HSMLibMountPath, path.Base(spec.LibraryPath)),
		TokenLabel: spec.TokenLabel,
		Pin:        pin,
	}
	confJSON, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling crypto11.conf: %w", err)
	}

	keyLabels := map[string]string{PKCS11ConfLabel: "crypto11.conf"}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(pkcs11ConfSecretFormat, instance.Name),
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, e.Client,
		secret,
		ensure.ControllerReference[*v1.Secret](instance, e.Client),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(keyLabels)), keyLabels),
		kubernetes.EnsureSecretData(true, map[string][]byte{"crypto11.conf": confJSON}),
	); err != nil {
		return fmt.Errorf("creating crypto11.conf secret: %w", err)
	}

	e.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "PKCS11ConfigCreated", "Created", "crypto11.conf secret created: %s", secret.Name)
	status.PKCS11ConfigRef = &rhtasv1alpha1.SecretKeySelector{
		Key:                  "crypto11.conf",
		LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: secret.Name},
	}
	return nil
}

func (e ensurePKCS11Config) handleRotation(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	existing, _ := kubernetes.FindSecret(ctx, e.Client, instance.Namespace, FulcioCALabel)
	if existing != nil {
		if err := labels.Remove(ctx, existing, e.Client, FulcioCALabel); err != nil {
			return e.Error(ctx, err, instance)
		}
		e.Recorder.Eventf(instance, nil, v1.EventTypeNormal,
			"PKCS11RotationCertPreserved", "Rotation",
			"Old cert label removed from %s", existing.Name)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: PKCS11ConfigCondition, Status: metav1.ConditionFalse,
		Reason: "Rotation", Message: "PKCS#11 configuration drift detected",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: CertCondition, Status: metav1.ConditionFalse, Reason: "Rotation",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type: constants.ReadyCondition, Status: metav1.ConditionFalse,
		Reason: state.Pending.String(), ObservedGeneration: instance.Generation,
	})

	e.Recorder.Eventf(instance, nil, v1.EventTypeNormal,
		"PKCS11RotationStarted", "Rotation",
		"Key rotation initiated, re-deploying Fulcio")

	return e.StatusUpdate(ctx, instance)
}

func (e ensurePKCS11Config) ensureInlineVolumes(
	ctx context.Context,
	instance *rhtasv1alpha1.Fulcio,
	spec *rhtasv1alpha1.PKCS11Config,
	status *rhtasv1alpha1.PKCS11Config,
	componentLabels map[string]string,
) error {
	for i, vol := range spec.InitContainer.Volumes {
		if len(vol.InlineData) == 0 {
			continue
		}

		volLabel := PKCS11VolLabelPrefix + vol.Name
		existing, err := kubernetes.FindConfigMap(ctx, e.Client, instance.Namespace, volLabel)
		if err == nil && existing != nil {
			if i < len(status.InitContainer.Volumes) {
				status.InitContainer.Volumes[i].ConfigMapName = existing.Name
				status.InitContainer.Volumes[i].InlineData = nil
			}
			continue
		}

		volLabels := map[string]string{volLabel: vol.Name}
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: fmt.Sprintf(pkcs11VolumesCMFormat, vol.Name, instance.Name),
				Namespace:    instance.Namespace,
			},
		}
		if _, err := kubernetes.CreateOrUpdate(ctx, e.Client,
			cm,
			ensure.ControllerReference[*v1.ConfigMap](instance, e.Client),
			ensure.Labels[*v1.ConfigMap](slices.Collect(maps.Keys(componentLabels)), componentLabels),
			ensure.Labels[*v1.ConfigMap](slices.Collect(maps.Keys(volLabels)), volLabels),
			kubernetes.EnsureConfigMapData(true, vol.InlineData),
		); err != nil {
			return fmt.Errorf("creating ConfigMap for volume %s: %w", vol.Name, err)
		}

		e.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "PKCS11VolumeConfigMapCreated", "Created",
			"ConfigMap created for volume %s: %s", vol.Name, cm.Name)

		if i < len(status.InitContainer.Volumes) {
			status.InitContainer.Volumes[i].ConfigMapName = cm.Name
			status.InitContainer.Volumes[i].InlineData = nil
		}
	}
	return nil
}
