package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ErrorBaseline = fmt.Errorf("failed to set declarative TUF baseline")

func NewBaselineAction() action.Action[*rhtasv1.Tuf] {
	return &baselineAction{}
}

type baselineAction struct {
	action.BaseAction
}

func (i baselineAction) Name() string {
	return "set declarative TUF baseline"
}

func (i baselineAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	return meta.IsStatusConditionTrue(instance.GetConditions(), tufConstants.RepositoryCondition) &&
		instance.Annotations[tufConstants.RepositoryVersionAnnotation] == tufConstants.TufVersionV1 &&
		instance.Status.TrustRoot == nil &&
		state.FromInstance(instance, constants.ReadyCondition) == state.Ready
}

func (i baselineAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	var err error
	published := &rhtasv1.Components{
		Ctlog:  make([]rhtasv1.TrustRootBinding, 0),
		Fulcio: make([]rhtasv1.TrustRootBindingWithOIDC, 0),
		Rekor:  make([]rhtasv1.TrustRootBinding, 0),
		Tsa:    make([]rhtasv1.TrustRootBinding, 0),
	}
	for _, key := range instance.Status.Keys {
		binding := rhtasv1.TrustRootBinding{
			TargetName: key.Name,
			SecretRef:  key.SecretRef,
		}
		switch key.Name {
		case rhtasv1.TufKeyCTFE:
			binding.URL, err = serviceresolver.ResolveExternalServiceUrl(ctx, i.Client, trustroot.FirstBinding(instance.Spec.Ctlog), instance.Namespace, &rhtasv1.CTlog{})
			if err != nil {
				err = errors.Join(ErrorBaseline, err)
				return i.Error(ctx, err, instance, metav1.Condition{
					Type:    tufConstants.TrustRootSynced,
					Status:  metav1.ConditionFalse,
					Reason:  state.Failure.String(),
					Message: err.Error(),
				})
			}
			published.Ctlog = append(published.Ctlog, binding)
		case rhtasv1.TufKeyFulcio:
			binding.URL, err = serviceresolver.ResolveExternalServiceUrl(ctx, i.Client, trustroot.FirstBinding(instance.Spec.Fulcio), instance.Namespace, &rhtasv1.Fulcio{})
			if err != nil {
				err = errors.Join(ErrorBaseline, err)
				return i.Error(ctx, err, instance, metav1.Condition{
					Type:    tufConstants.TrustRootSynced,
					Status:  metav1.ConditionFalse,
					Reason:  state.Failure.String(),
					Message: err.Error(),
				})
			}
			var oidcIssuers []string
			oidcIssuers, err = trustroot.LoadFulcioIssuers(ctx, i.Client, instance.Namespace, instance.Spec.Fulcio, &rhtasv1.Fulcio{})
			if err != nil {
				err = errors.Join(ErrorBaseline, err)
				return i.Error(ctx, err, instance, metav1.Condition{
					Type:    tufConstants.TrustRootSynced,
					Status:  metav1.ConditionFalse,
					Reason:  state.Failure.String(),
					Message: err.Error(),
				})
			}
			published.Fulcio = append(published.Fulcio, rhtasv1.TrustRootBindingWithOIDC{
				TrustRootBinding: binding,
				OIDCIssuers:      oidcIssuers,
			})
		case rhtasv1.TufKeyRekor:
			binding.URL, err = serviceresolver.ResolveExternalServiceUrl(ctx, i.Client, trustroot.FirstBinding(instance.Spec.Rekor), instance.Namespace, &rhtasv1.Rekor{})
			if err != nil {
				err = errors.Join(ErrorBaseline, err)
				return i.Error(ctx, err, instance, metav1.Condition{
					Type:    tufConstants.TrustRootSynced,
					Status:  metav1.ConditionFalse,
					Reason:  state.Failure.String(),
					Message: err.Error(),
				})
			}
			published.Rekor = append(published.Rekor, binding)
		case rhtasv1.TufKeyTSA:
			if instance.Spec.Tsa == nil {
				continue
			}
			binding.URL, err = serviceresolver.ResolveExternalServiceUrl(ctx, i.Client, trustroot.FirstBinding(*instance.Spec.Tsa), instance.Namespace, &rhtasv1.TimestampAuthority{})
			if err != nil {
				err = errors.Join(ErrorBaseline, err)
				return i.Error(ctx, err, instance, metav1.Condition{
					Type:    tufConstants.TrustRootSynced,
					Status:  metav1.ConditionFalse,
					Reason:  state.Failure.String(),
					Message: err.Error(),
				})
			}
			published.Tsa = append(published.Tsa, binding)
		}
	}

	instance.Status.TrustRoot = &rhtasv1.TrustRootStatus{Published: published}
	instance.Status.Keys = nil

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               tufConstants.TrustRootSynced,
		Status:             metav1.ConditionTrue,
		Reason:             tufConstants.Synchronized,
		Message:            "TUF baseline set",
		ObservedGeneration: instance.Generation,
	})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
