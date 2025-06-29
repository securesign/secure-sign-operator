package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
)

func NewGeneratePasswordAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &generatePasswordAction{}
}

type generatePasswordAction struct {
	action.BaseAction
}

func (i generatePasswordAction) Name() string {
	return "redis-config"
}

func (i generatePasswordAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Pending || c.Reason == constants.Ready) &&
		instance.Status.SearchIndex.DbPasswordRef == nil && enabled(instance)
}

func (i generatePasswordAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)
	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)

	obj := &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    instance.Namespace,
			GenerateName: fmt.Sprintf("redis-password-%s", instance.Name),
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, obj,
		ensure.ControllerReference[*core.Secret](instance, i.Client),
		ensure.Labels[*core.Secret](slices.Collect(maps.Keys(labels)), labels),
		kubernetes.EnsureSecretData(true, map[string][]byte{"password": utils.GeneratePassword(8)}),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s db password Secret: %w", obj.Name, err), instance,
			metav1.Condition{
				Type:    actions.RedisCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	instance.Status.SearchIndex.DbPasswordRef = &rhtasv1alpha1.SecretKeySelector{
		LocalObjectReference: rhtasv1alpha1.LocalObjectReference{Name: obj.Name},
		Key:                  "password",
	}
	i.Recorder.Eventf(instance, core.EventTypeNormal, "RedisSecretCreated", "Secret with redis password created: %s", obj.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               actions.RedisCondition,
		Status:             metav1.ConditionFalse,
		Reason:             constants.Pending,
		Message:            "Redis password created",
		ObservedGeneration: instance.Generation,
	})
	r := i.StatusUpdate(ctx, instance)
	if action.IsSuccess(r) {
		i.cleanup(ctx, instance, labels)
	}
	return r
}

func (i generatePasswordAction) cleanup(ctx context.Context, instance *rhtasv1alpha1.Rekor, configLabels map[string]string) {
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
			i.Recorder.Eventf(instance, core.EventTypeWarning, "RedisSecretDeleted", "Unable to delete Secret: %s", partialSecret.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with redis configuration", "Name", partialSecret.Name)
		i.Recorder.Eventf(instance, core.EventTypeNormal, "RedisSecretDeleted", "Secret with redis configuration deleted: %s", partialSecret.Name)
	}
}
