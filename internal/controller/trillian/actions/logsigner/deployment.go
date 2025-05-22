package logsigner

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
	"github.com/securesign/operator/internal/utils/tls"

	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.LogSignerComponentName, actions.LogsignerDeploymentName, instance.Name)

	caPath, err := tls.CAPath(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.LogsignerDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		trillianUtils.EnsureServerDeployment(instance, images.Registry.Get(images.TrillianLogSigner), actions.LogsignerDeploymentName, actions.RBACName, labels, "--force_master=true"),
		ensure.ControllerReference[*apps.Deployment](instance, i.Client),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), "wait-for-trillian-db", actions.LogsignerDeploymentName),
		ensure.Optional(
			trillianUtils.UseTLSDb(instance),
			trillianUtils.WithTlsDB(instance, caPath, actions.LogsignerDeploymentName),
		),
		ensure.Optional(
			statusTLS(instance).CertRef != nil,
			trillianUtils.EnsureTLS(statusTLS(instance), actions.LogsignerDeploymentName),
		),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian LogSigner: %w", err), instance, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Deployment created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
