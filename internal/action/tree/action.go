package tree

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	"github.com/securesign/operator/internal/utils/tls"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const jobKind = "Job"

func NewResolveTreeAction[T tlsAwareObject](component string, wrapper func(T) *wrapper[T]) action.Action[T] {
	return &resolveTree[T]{
		component:       component,
		treeDisplayName: fmt.Sprintf("%s-tree", component),
		wrapper:         wrapper,
	}
}

type resolveTree[T tlsAwareObject] struct {
	action.BaseAction
	component       string
	treeDisplayName string
	wrapper         func(T) *wrapper[T]
}

func (i resolveTree[T]) Name() string {
	return "resolve tree"
}

// resourceBase returns the name prefix used for this instance's createtree RBAC, Job,
// and ConfigMap. When the instance tracks more than one tree (wrapper.GetDiscriminator
// returns a non-empty value, e.g. a CTLog shard prefix), the prefix is used within the
// name so that rotating the active tree can't collide with a previous tree's resources.
func (i resolveTree[T]) resourceBase(instance T) string {
	discriminator := i.wrapper(instance).GetDiscriminator()
	if discriminator == "" {
		return i.component
	}
	sum := sha256.Sum256([]byte(discriminator))
	return fmt.Sprintf("%s-%s", i.component, hex.EncodeToString(sum[:8]))
}

func (i resolveTree[T]) CanHandle(ctx context.Context, instance T) bool {
	wrapped := i.wrapper(instance)

	switch {
	case wrapped.GetStatusTreeID() == nil:
		return true
	case wrapped.GetTreeID() != nil:
		return !equality.Semantic.DeepEqual(wrapped.GetTreeID(), wrapped.GetStatusTreeID())
	default:
		return !meta.IsStatusConditionTrue(instance.GetConditions(), JobCondition)
	}
}

// JobCondition was added on 1.3.0. We need to handle missing condition for backward compatibility
func (i resolveTree[T]) handleMissingCondition(ctx context.Context, instance T) *action.Result {
	conditions := instance.GetConditions()
	wrapped := i.wrapper(instance)
	if meta.FindStatusCondition(conditions, JobCondition) == nil && wrapped.GetStatusTreeID() != nil && *wrapped.GetStatusTreeID() != int64(0) {
		// tree is already initialized, just add JobCondition
		instance.SetCondition(metav1.Condition{
			Type:   JobCondition,
			Status: metav1.ConditionTrue,
			Reason: state.Ready.String(),
		})
		i.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "ExistingTrillianTreeFound", "Discovered", "Existing Trillian tree found: %d", wrapped.GetStatusTreeID())
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return nil
}

func (i resolveTree[T]) handleManual(ctx context.Context, instance T) *action.Result {
	wrapped := i.wrapper(instance)

	if wrapped.GetTreeID() != nil && *wrapped.GetTreeID() != int64(0) {
		wrapped.SetStatusTreeID(wrapped.GetTreeID())
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleRbac(ctx context.Context, instance T) *action.Result {
	rbacName := fmt.Sprintf(RBACNameMask, i.resourceBase(instance))

	labels := labels.For("createtree", i.component, instance.GetName())

	sa, err := traits.Apply(
		corev1ac.ServiceAccount(rbacName, instance.GetNamespace()),
		traits.ControllerReference[*corev1ac.ServiceAccountApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ServiceAccountApplyConfiguration](labels),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build SA: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, sa, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create SA: %w", err), instance)
	}

	role, err := traits.Apply(
		rbacv1ac.Role(rbacName, instance.GetNamespace()),
		traits.ControllerReference[*rbacv1ac.RoleApplyConfiguration](instance, i.Client),
		traits.Labels[*rbacv1ac.RoleApplyConfiguration](labels),
		traits.RoleRules(
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"patch"},
			}),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build Role: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, role, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Role: %w", err), instance)
	}

	binding, err := traits.Apply(
		rbacv1ac.RoleBinding(rbacName, instance.GetNamespace()),
		traits.ControllerReference[*rbacv1ac.RoleBindingApplyConfiguration](instance, i.Client),
		traits.Labels[*rbacv1ac.RoleBindingApplyConfiguration](labels),
		traits.RoleBinding(
			rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     rbacName,
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: rbacName, Namespace: instance.GetNamespace()},
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build RoleBinding: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, binding, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create RoleBinding: %w", err), instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleConfigMap(ctx context.Context, instance T) *action.Result {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(configMapResultMask, i.resourceBase(instance), instance.GetName()), Namespace: instance.GetNamespace()},
	}
	if err := i.Client.Get(ctx, client.ObjectKeyFromObject(configMap), configMap); client.IgnoreNotFound(err) != nil {
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	changed, err := i.applyConfigMap(ctx, instance, configMap)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not apply ConfigMap: %w", err), instance)
	}
	if changed {
		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: fmt.Sprintf("ConfigMap `%s` applied", configMap.Name),
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func (i resolveTree[T]) applyConfigMap(ctx context.Context, instance T, configMap *corev1.ConfigMap) (bool, error) {
	cm, err := traits.Apply(
		corev1ac.ConfigMap(configMap.Name, configMap.Namespace),
		traits.ControllerReference[*corev1ac.ConfigMapApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ConfigMapApplyConfiguration](labels.For("createtree", i.component, instance.GetName())),
	)
	if err != nil {
		return false, err
	}

	// Both steps use the same field manager, so retain the Job reference on
	// every apply. The Job writes data, which the operator leaves unowned.
	for _, ref := range configMap.GetOwnerReferences() {
		if ref.Kind != jobKind {
			continue
		}
		owner, err := traits.ConvertViaJSON[metav1.OwnerReference, metav1ac.OwnerReferenceApplyConfiguration](ref)
		if err != nil {
			return false, err
		}
		cm.WithOwnerReferences(owner)
	}
	return kubernetes.Apply(ctx, i.Client, cm, configMap)
}

func (i resolveTree[T]) handleJob(ctx context.Context, instance T) *action.Result {
	var err error
	var trillUrl string
	wrapped := i.wrapper(instance)

	labels := labels.For("createtree", i.component, instance.GetName())

	configMapName := fmt.Sprintf(configMapResultMask, i.resourceBase(instance), instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.RequeueAfter(5 * time.Second)
		}
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	for _, ref := range configMap.GetOwnerReferences() {
		if ref.Kind == jobKind {
			return i.Continue()
		}
	}

	trillianService := wrapped.GetTrillianService()

	trillHost, trillPort, err := serviceresolver.ResolveInternalGrpcService(ctx, i.Client, *trillianService, instance.GetNamespace(), &rhtasv1.Trillian{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve trillian service: %w", err), instance)
	}
	trillUrl = fmt.Sprintf("%s:%s", trillHost, trillPort)
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	jobConfig, err := traits.Apply(
		batchv1ac.Job("", instance.GetNamespace()).WithGenerateName(fmt.Sprintf(JobNameMask, i.resourceBase(instance))),
		traits.ControllerReference[*batchv1ac.JobApplyConfiguration](instance, i.Client),
		traits.Labels[*batchv1ac.JobApplyConfiguration](labels),
		i.ensureJob(configMapName, trillUrl, i.treeDisplayName),
		traits.WithJobPodSpecTraits(
			traits.SecurityContexts(createTreeContainerName),
			traits.GODEBUG(instance.GetAnnotations(), createTreeContainerName),
			traits.TrustedCA(instance.GetTrustedCA(), createTreeContainerName),
			traits.Optional(instance.GetTrustedCA() != nil || kubernetes.IsOpenShift(), func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
				caPath, err := tls.CAPath(ctx, i.Client, instance)
				if err != nil {
					return fmt.Errorf("could not get CA path: %w", err)
				}
				kubernetes.FindContainerByName(podSpec, createTreeContainerName).WithEnv(corev1ac.EnvVar().WithName("EXTRA_ARGS").WithValue(fmt.Sprintf("--tls_cert_file=%s", caPath)))
				return nil
			}),
			traits.ServiceAccount(fmt.Sprintf(RBACNameMask, i.resourceBase(instance))),
		))
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build tree job: %w", err), instance)
	}
	job, err := traits.ConvertViaJSON[*batchv1ac.JobApplyConfiguration, batchv1.Job](jobConfig)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not convert tree job: %w", err), instance)
	}
	if err = kubernetes.Create(ctx, i.Client, job); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create createtree job: %w", err), instance,
			metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Creating.String(),
				Message: err.Error(),
			})
	}

	if err = controllerutil.SetOwnerReference(job, configMap, i.Client.Scheme()); err == nil {
		_, err = i.applyConfigMap(ctx, instance, configMap)
	}
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not apply Job owner reference to %s ConfigMap: %w", configMapName, err), instance,
			metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Creating.String(),
				Message: err.Error(),
			})
	}

	instance.SetCondition(metav1.Condition{
		Type:    JobCondition,
		Status:  metav1.ConditionFalse,
		Reason:  state.Initialize.String(),
		Message: "createtree job created",
	})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func (i resolveTree[T]) handleJobFinished(ctx context.Context, instance T) *action.Result {
	var (
		jobName string
		err     error
	)

	configMapName := fmt.Sprintf(configMapResultMask, i.resourceBase(instance), instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.RequeueAfter(5 * time.Second)
		}
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	for _, ref := range configMap.GetOwnerReferences() {
		if ref.Kind == jobKind {
			jobName = ref.Name
			break
		}
	}
	if jobName == "" {
		return i.RequeueAfter(5 * time.Second)
	}

	j, err := job.GetJob(ctx, i.Client, instance.GetNamespace(), jobName)
	if client.IgnoreNotFound(err) != nil {
		return i.Error(ctx, err, instance)
	}

	if j == nil {
		return i.RequeueAfter(5 * time.Second)
	}
	i.Logger.V(1).Info("createtree job is already present.", "Succeeded", j.Status.Succeeded, "Failures", j.Status.Failed)

	if !job.IsCompleted(*j) {
		return i.RequeueAfter(5 * time.Second)
	}

	if job.IsFailed(*j) {
		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: ErrJobFailed.Error(),
		})
		return i.Error(ctx, reconcile.TerminalError(ErrJobFailed), instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleExtractJobResult(ctx context.Context, instance T) *action.Result {
	wrapped := i.wrapper(instance)

	configMapName := fmt.Sprintf(configMapResultMask, i.resourceBase(instance), instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.RequeueAfter(5 * time.Second)
		}
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	if result, ok := configMap.Data[configMapResultField]; ok && result != "" {
		treeID, err := strconv.ParseInt(result, 10, 64)
		if err != nil {
			return i.Error(ctx, reconcile.TerminalError(err), instance)
		}

		wrapped.SetStatusTreeID(&treeID)
		instance.SetCondition(metav1.Condition{
			Type:   JobCondition,
			Status: metav1.ConditionTrue,
			Reason: state.Ready.String(),
		})
		i.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "TrillianTreeCreated", "Created", "New Trillian tree created: %d", treeID)
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		i.Logger.V(1).Info("ConfigMap not ready or data is empty, requeuing reconciliation")
		return i.RequeueAfter(5 * time.Second)
	}
}

func (i resolveTree[T]) ensureJob(cfgName, adminServer, displayName string) traits.Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
		container := corev1ac.Container().
			WithName(createTreeContainerName).
			WithImage(images.Registry.Get(images.TrillianCreateTree)).
			WithCommand("/bin/sh", "-c").
			WithArgs(string(jobScript)).
			WithEnv(
				corev1ac.EnvVar().WithName("CONFIGMAP_NAME").WithValue(cfgName),
				corev1ac.EnvVar().WithName("ADMIN_SERVER").WithValue(adminServer),
				corev1ac.EnvVar().WithName("DISPLAY_NAME").WithValue(displayName),
			)

		podSpec := corev1ac.PodSpec().
			WithRestartPolicy(corev1.RestartPolicyOnFailure).
			WithContainers(container)

		job.WithSpec(batchv1ac.JobSpec().
			WithParallelism(1).
			WithCompletions(1).
			WithActiveDeadlineSeconds(600).
			WithBackoffLimit(5).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(job.Labels).WithSpec(podSpec)))

		return nil
	}
}

func (i resolveTree[T]) Handle(ctx context.Context, instance T) *action.Result {
	result := i.handleMissingCondition(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleManual(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleRbac(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleConfigMap(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleJob(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleJobFinished(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleExtractJobResult(ctx, instance)
	if result != nil {
		return result
	}

	return i.Continue()
}
