package logsigner

import (
	"context"
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"

	"github.com/securesign/operator/internal/controller/trillian/actions"
	trillianUtils "github.com/securesign/operator/internal/controller/trillian/utils"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

func NewDeployAction() action.Action[*rhtasv1.Trillian] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Trillian) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {

	caPath, err := tls.CAPath(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	}

	dp, err := traits.Apply(
		appsv1ac.Deployment(actions.LogsignerDeploymentName, instance.Namespace),
		traits.ControllerReference[*appsv1ac.DeploymentApplyConfiguration](instance, i.Client),
		traits.Labels[*appsv1ac.DeploymentApplyConfiguration](labels.For(actions.LogSignerComponentName, actions.LogsignerDeploymentName, instance.Name)),
		trillianUtils.EnsureDeployment(instance, images.Registry.Get(images.TrillianLogSigner), actions.LogsignerDeploymentName),
		func(dp *appsv1ac.DeploymentApplyConfiguration) error {
			kubernetes.FindContainerByName(dp.Spec.Template.Spec, actions.LogsignerDeploymentName).
				WithArgs("--election_system=k8s", "--lock_namespace=$(NAMESPACE)", "--lock_holder_identity=$(POD_NAME)", "--master_hold_interval=5s", "--master_hold_jitter=15s")
			return nil
		},
		traits.PodRequirements(instance.Spec.LogSigner.PodRequirements, actions.LogsignerDeploymentName),
		traits.WithPodSpecTraits(
			traits.Proxy([]string{actions.LogsignerDeploymentName}),
			traits.GODEBUG(instance.GetAnnotations(), actions.LogsignerDeploymentName),
			traits.TrustedCA(instance.GetTrustedCA(), actions.LogsignerDeploymentName),
			trillianUtils.EnsureDB(instance, actions.LogsignerDeploymentName, caPath),
			traits.Auth(trillianUtils.WithDbAuth(instance.Spec.Auth, instance.Status.Db.DatabaseSecretRef), actions.LogsignerDeploymentName, trillianUtils.WaitForDBInitContainer),
			traits.SecurityContexts(actions.LogsignerDeploymentName),
			traits.ServiceAccount(actions.RBACSignerName),
			traits.Optional(
				statusTLS(instance).CertRef != nil,
				trillianUtils.EnsureTLS(statusTLS(instance), actions.LogsignerDeploymentName),
			),
			traits.PodExtensions(instance.Spec.PodExtensions, actions.LogsignerDeploymentName),
		),
	)

	if err != nil {
		if errors.Is(err, traits.ErrPodExtensionConflict) {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
		return i.Error(ctx, fmt.Errorf("could not build Trillian LogSigner deployment: %w", err), instance, metav1.Condition{
			Type: actions.SignerCondition, Status: metav1.ConditionFalse,
			Reason: state.Failure.String(), Message: err.Error(),
		})
	}

	changed, err := kubernetes.Apply(ctx, i.Client, dp, &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: actions.LogsignerDeploymentName, Namespace: instance.Namespace},
	})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Trillian LogSigner: %w", err), instance, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.SignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Deployment created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}
