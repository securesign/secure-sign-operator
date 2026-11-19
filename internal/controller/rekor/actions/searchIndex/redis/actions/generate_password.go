package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func NewGeneratePasswordAction() action.Action[*rhtasv1.Rekor] {
	return &generatePasswordAction{}
}

type generatePasswordAction struct {
	action.BaseAction
}

func (i generatePasswordAction) Name() string {
	return "redis-config"
}

func (i generatePasswordAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return enabled(instance) &&
		instance.Status.SearchIndex.DbPasswordRef == nil &&
		state.FromInstance(instance, constants.ReadyCondition) >= state.Pending
}

func (i generatePasswordAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	secretLabels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)
	secretName := fmt.Sprintf("redis-password-%s", instance.Name)
	secretConfig, err := traits.Apply(
		corev1ac.Secret("", instance.Namespace).
			WithGenerateName(secretName).
			WithImmutable(true).
			WithData(map[string][]byte{"password": utils.GeneratePassword(8)}),
		traits.ControllerReference[*corev1ac.SecretApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.SecretApplyConfiguration](secretLabels),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build %s db password Secret: %w", secretName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	secret, err := kubernetes.Create(ctx, i.Client, secretConfig)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s db password Secret: %w", secretName, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			},
		)
	}

	instance.Status.SearchIndex.DbPasswordRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: secret.GetName()},
		Key:                  "password",
	}
	i.Recorder.Eventf(instance, secret, core.EventTypeNormal, "RedisSecretCreated", "Created", "Secret with redis password created: %s", secret.GetName())
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               actions.RedisCondition,
		Status:             metav1.ConditionFalse,
		Reason:             state.Pending.String(),
		Message:            "Redis password created",
		ObservedGeneration: instance.Generation,
	})
	changed, err := i.PersistStatus(ctx, instance)
	if err != nil {
		return i.Error(ctx, err, instance)
	}
	i.cleanup(ctx, instance, secretLabels)
	if changed {
		return i.Return()
	}
	return i.Continue()
}

func (i generatePasswordAction) cleanup(ctx context.Context, instance *rhtasv1.Rekor, configLabels map[string]string) {
	if instance.Status.SearchIndex.DbPasswordRef == nil || instance.Status.SearchIndex.DbPasswordRef.Name == "" {
		i.Logger.Error(errors.New("new Secret name is empty"), "unable to clean old objects", "namespace", instance.Namespace)
		return
	}

	// try to discover existing secrets and clear them out
	partialSecrets, err := kubernetes.ListSecrets(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(configLabels).String())
	if err != nil {
		i.Logger.Error(err, "problem with listing secrets", "namespace", instance.Namespace)
		return
	}
	for _, partialSecret := range partialSecrets.Items {
		if partialSecret.Name == instance.Status.SearchIndex.DbPasswordRef.Name {
			continue
		}

		err = i.Client.Delete(ctx, &core.Secret{ObjectMeta: metav1.ObjectMeta{Name: partialSecret.Name, Namespace: partialSecret.Namespace}})
		if err != nil {
			i.Logger.Error(err, "unable to delete Secret", "namespace", instance.Namespace, "name", partialSecret.Name)
			i.Recorder.Eventf(instance, nil, core.EventTypeWarning, "RedisSecretDeleted", "CleanupFailed", "Unable to delete Secret: %s", partialSecret.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with redis configuration", "Name", partialSecret.Name)
		i.Recorder.Eventf(instance, nil, core.EventTypeNormal, "RedisSecretDeleted", "Deleted", "Secret with redis configuration deleted: %s", partialSecret.Name)
	}
}
