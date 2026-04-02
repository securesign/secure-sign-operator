package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	jobUtils "github.com/securesign/operator/internal/utils/kubernetes/job"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apilabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const tufVersionAnnotation = "rhtas.redhat.com/tuf-version"

func NewMigrationJobAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &migrationJobAction{}
}

type migrationJobAction struct {
	action.BaseAction
}

func (i migrationJobAction) Name() string {
	return "migration job"
}

func (i migrationJobAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	switch tuf.Annotations[tufVersionAnnotation] {
	case "v1":
		return false
	default:
		s := state.FromInstance(tuf, constants.ReadyCondition)
		return s == state.Initialize || s == state.Ready
	}
}

func (i migrationJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	if instance.Spec.RootKeySecretRef != nil && instance.Spec.RootKeySecretRef.Name != "" {
		if _, err := kubernetes.GetSecret(i.Client, instance.Namespace, instance.Spec.RootKeySecretRef.Name); err != nil {
			if errors.IsNotFound(err) {
				i.Logger.Info("Root key secret not found", "secret", instance.Spec.RootKeySecretRef.Name)
				return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("cannot migrate TUF: root key secret %s not found: %w", instance.Spec.RootKeySecretRef.Name, err)), instance)
			}
			return i.Error(ctx, err, instance)
		}
	} else {
		i.Logger.Info("root key secret not specified")
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("cannot migrate TUF: root key secret not specified")), instance)
	}

	jobLabels := labels.ForResource(tufConstants.ComponentName, tufConstants.MigrationJobName, instance.Name, instance.Status.PvcName)
	jobList := &batchv1.JobList{}
	selector := apilabels.SelectorFromSet(jobLabels)
	if err := kubernetes.FindByLabelSelector(ctx, i.Client, jobList, instance.Namespace, selector.String()); err != nil {
		return i.Error(ctx, err, instance)
	}

	switch {
	case len(jobList.Items) > 1:
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("multiple %s jobs present", tufConstants.MigrationJobName)), instance)
	case len(jobList.Items) == 1:
		return i.jobPresent(ctx, &jobList.Items[0], instance)
	default:
		return i.ensureMigrationJob(ctx, jobLabels, instance)
	}
}

func (i migrationJobAction) jobPresent(ctx context.Context, job *batchv1.Job, instance *rhtasv1alpha1.Tuf) *action.Result {
	i.Logger.Info("Tuf migration job is present.", "Succeeded", job.Status.Succeeded, "Failures", job.Status.Failed)
	if jobUtils.IsCompleted(*job) {
		//scale the deployment back in any case
		// can't use kubernetes.CreateOrUpdate because it is paused by annotation
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
			deployment := &appsv1.Deployment{}
			if err := i.Client.Get(ctx, types.NamespacedName{
				Namespace: instance.Namespace,
				Name:      tufConstants.DeploymentName,
			}, deployment); err != nil {
				return err
			}
			if err := ensure.Annotations[*appsv1.Deployment]([]string{annotations.PausedReconciliation}, map[string]string{})(deployment); err != nil {
				return err
			}
			if err := i.scaleDeployment(instance.Spec.Replicas)(deployment); err != nil {
				return err
			}
			return i.Client.Update(ctx, deployment)
		}); err != nil {
			return i.Error(ctx, err, instance)
		}

		if !jobUtils.IsFailed(*job) {
			i.Recorder.Event(instance, corev1.EventTypeNormal, "TUFMigrationJob", "TUF migration job passed")

			//annotate self to signal that the migration is complete
			var (
				result controllerutil.OperationResult
				err    error
			)
			if result, err = kubernetes.CreateOrUpdate(ctx, i.Client, instance,
				ensure.Annotations[*rhtasv1alpha1.Tuf]([]string{tufVersionAnnotation}, map[string]string{tufVersionAnnotation: "v1"}),
			); err != nil {
				return i.Error(ctx, err, instance)
			}

			if result != controllerutil.OperationResultNone {
				return i.Requeue()
			} else {
				return i.Continue()
			}
		} else {
			err := fmt.Errorf("tuf-repository-migration job failed")
			i.Recorder.Event(instance, corev1.EventTypeWarning, "TUFMigrationJob", err.Error())
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}
	} else {
		// job not completed yet
		return i.Requeue()
	}
}

func (i migrationJobAction) ensureMigrationJob(ctx context.Context, labels map[string]string, instance *rhtasv1alpha1.Tuf) *action.Result {
	i.Recorder.Event(instance, corev1.EventTypeNormal, "TUFMigrationJob", "Starting TUF migration")

	// if the ctlog prefix is not specified, use the default prefix (backward compatibility)
	if instance.Spec.Ctlog.Prefix == "" {
		instance.Spec.Ctlog.Prefix = ctlogUtils.DefaultPrefix
	}

	if err := utils.ResolveServiceAddress(ctx, i.Client, instance); err != nil {
		return i.Error(ctx, fmt.Errorf("fail to resolve service url: %w", err), instance)
	}
	oidcIssuers := utils.ResolveOIDCIssuers(ctx, i.Client, instance.Namespace)

	if _, err := kubernetes.CreateOrUpdate(ctx, i.Client,
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: tufConstants.MigrationJobName + "-",
				Namespace:    instance.Namespace,
			},
		},
		// use init job RBAC and do not introduce new RBAC for migration job
		utils.EnsureTufMigrationJob(instance, tufConstants.RBACInitJobName, labels, oidcIssuers),
		ensure.ControllerReference[*batchv1.Job](instance, i.Client),
		ensure.Labels[*batchv1.Job](slices.Collect(maps.Keys(labels)), labels),
		func(object *batchv1.Job) error {
			ensure.SetProxyEnvs(object.Spec.Template.Spec.Containers)
			return nil
		},
		func(object *batchv1.Job) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
		},
	); err != nil {
		//do not terminate - retry with exponential backoff
		return i.Error(ctx, fmt.Errorf("could not create TUF migration job: %w", err),
			instance, metav1.Condition{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String(), Message: "TUF migration job creation failed"})
	}

	i.Recorder.Event(instance, corev1.EventTypeNormal, "TUFMigrationJob", "Tuf migration job created.")

	//scale the deployment to 0 to free up PVC access for migration job
	deployment := &appsv1.Deployment{}
	if err := i.Client.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      tufConstants.DeploymentName,
	}, deployment); err != nil {
		return i.Error(ctx, err, instance)
	}
	if _, err := kubernetes.CreateOrUpdate(ctx, i.Client, deployment,
		i.scaleDeployment(ptr.To[int32](0)),
		ensure.Annotations[*appsv1.Deployment]([]string{annotations.PausedReconciliation}, map[string]string{annotations.PausedReconciliation: "true"}),
	); err != nil {
		return i.Error(ctx, err, instance)
	}
	return i.Requeue()

}

func (i migrationJobAction) scaleDeployment(replicas *int32) func(*appsv1.Deployment) error {
	return func(dp *appsv1.Deployment) error {
		dp.Spec.Replicas = replicas
		return nil
	}
}
