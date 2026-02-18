package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekor "github.com/securesign/operator/internal/controller/rekor/actions"
	tsa "github.com/securesign/operator/internal/controller/tsa/actions"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	jobUtils "github.com/securesign/operator/internal/utils/kubernetes/job"
	v2 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	v3 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apilabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewInitJobAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &initJobAction{}
}

type initJobAction struct {
	action.BaseAction
}

func (i initJobAction) Name() string {
	return "tuf-init job"
}

func (i initJobAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Tuf) bool {
	return !meta.IsStatusConditionTrue(instance.GetConditions(), tufConstants.RepositoryCondition) &&
		state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i initJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	jobLabels := labels.ForResource(tufConstants.ComponentName, tufConstants.InitJobName, instance.Name, instance.Status.PvcName)
	initJobList := &v2.JobList{}
	selector := apilabels.SelectorFromSet(jobLabels)
	if err := kubernetes.FindByLabelSelector(ctx, i.Client, initJobList, instance.Namespace, selector.String()); err != nil {
		return i.Error(ctx, err, instance)
	}

	switch {
	case len(initJobList.Items) > 1:
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("multiple %s jobs present", tufConstants.InitJobName)), instance)
	case len(initJobList.Items) == 1:
		return i.jobPresent(ctx, &initJobList.Items[0], instance)
	default:
		return i.ensureInitJob(ctx, jobLabels, instance)
	}
}

func (i initJobAction) jobPresent(ctx context.Context, job *v2.Job, instance *rhtasv1alpha1.Tuf) *action.Result {
	i.Logger.Info("Tuf tuf-repository-init is present.", "Succeeded", job.Status.Succeeded, "Failures", job.Status.Failed)
	if jobUtils.IsCompleted(*job) {
		if !jobUtils.IsFailed(*job) {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    tufConstants.RepositoryCondition,
				Status:  metav1.ConditionTrue,
				Reason:  state.Ready.String(),
				Message: "tuf-repository-init job passed",
			})
			return i.StatusUpdate(ctx, instance)
		} else {
			err := fmt.Errorf("tuf-repository-init job failed")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    tufConstants.RepositoryCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
	} else {
		// job not completed yet
		return i.Requeue()
	}
}

func (i initJobAction) ensureInitJob(ctx context.Context, labels map[string]string, instance *rhtasv1alpha1.Tuf) *action.Result {
	i.resolveServiceURLs(ctx, instance)
	oidcIssuers := i.resolveOIDCIssuers(ctx, instance.Namespace)

	if _, err := kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: tufConstants.InitJobName + "-",
				Namespace:    instance.Namespace,
			},
		},
		utils.EnsureTufInitJob(instance, tufConstants.RBACInitJobName, labels, oidcIssuers),
		ensure.ControllerReference[*v2.Job](instance, i.Client),
		ensure.Labels[*v2.Job](slices.Collect(maps.Keys(labels)), labels),
		func(object *v2.Job) error {
			ensure.SetProxyEnvs(object.Spec.Template.Spec.Containers)
			return nil
		},
		func(object *v2.Job) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF init job: %w", err), instance)
	}

	i.Recorder.Event(instance, v1.EventTypeNormal, "JobCreated", "Tuf init-repository job created.")
	return i.Requeue()
}

func (i initJobAction) resolveServiceURLs(ctx context.Context, instance *rhtasv1alpha1.Tuf) {
	if instance.Spec.SigningConfigURLMode == rhtasv1alpha1.SigningConfigURLInternal {
		return
	}
	services := []struct {
		address     *string
		ingressName string
		suffix      string
	}{
		{&instance.Spec.Fulcio.Address, fulcio.DeploymentName, ""},
		{&instance.Spec.Rekor.Address, rekor.ServerDeploymentName, ""},
		{&instance.Spec.Tsa.Address, tsa.DeploymentName, tsa.TimestampPath},
	}
	for _, svc := range services {
		if *svc.address == "" {
			if url := i.resolveURLFromIngress(ctx, svc.ingressName, instance.Namespace); url != "" {
				*svc.address = url + svc.suffix
			}
		}
	}
}

func (i initJobAction) resolveURLFromIngress(ctx context.Context, ingressName, namespace string) string {
	ingress := &v3.Ingress{}
	if err := i.Client.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: namespace}, ingress); err != nil {
		return ""
	}
	if len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].Host == "" {
		return ""
	}
	protocol := "http"
	if len(ingress.Spec.TLS) > 0 {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s", protocol, ingress.Spec.Rules[0].Host)
}

func (i initJobAction) resolveOIDCIssuers(ctx context.Context, namespace string) []string {
	fulcioList := &rhtasv1alpha1.FulcioList{}
	if err := i.Client.List(ctx, fulcioList, client.InNamespace(namespace)); err != nil {
		return nil
	}
	if len(fulcioList.Items) == 0 {
		return nil
	}

	fulcioInstance := &fulcioList.Items[0]
	var issuers []string
	for _, oidc := range fulcioInstance.Spec.Config.OIDCIssuers {
		if oidc.IssuerURL != "" {
			issuers = append(issuers, oidc.IssuerURL)
		} else if oidc.Issuer != "" {
			issuers = append(issuers, oidc.Issuer)
		}
	}
	return issuers
}
