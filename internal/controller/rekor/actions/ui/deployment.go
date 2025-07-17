package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"

	"github.com/securesign/operator/internal/controller/rekor/actions"
	v2 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)
	labels := labels.For(actions.UIComponentName, actions.SearchUiDeploymentName, instance.Name)
	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.SearchUiDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureUIDeployment(instance, actions.RBACUIName, labels),
		deployment.PodRequirements(instance.Spec.RekorSearchUI.PodRequirements, actions.SearchUiDeploymentName),
		ensure.ControllerReference[*v2.Deployment](instance, i.Client),
		ensure.Labels[*v2.Deployment](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Rekor search UI: %w", err), instance,
			metav1.Condition{
				Type:    actions.UICondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			},
		)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureUIDeployment(instance *rhtasv1alpha1.Rekor, sa string, labels map[string]string) func(*v2.Deployment) error {
	return func(dp *v2.Deployment) error {
		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.SearchUiDeploymentName)
		container.Image = images.Registry.Get(images.RekorSearchUi)

		env := kubernetes.FindEnvByNameOrCreate(container, "NEXT_PUBLIC_REKOR_DEFAULT_DOMAIN")
		env.Value = instance.Status.Url

		serverPort := kubernetes.FindPortByNameOrCreate(container, "3000-tcp")
		serverPort.ContainerPort = 3000

		return nil
	}
}
