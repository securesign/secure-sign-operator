package tree

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	"github.com/securesign/operator/internal/utils/tls"
	ensureTls "github.com/securesign/operator/internal/utils/tls/ensure"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// createTreeRetryBaseDelay is the base retry delay; attempt N waits base*N.
const createTreeRetryBaseDelay = 20 * time.Second

// createTreeMaxAttempts bounds failed-Job retries before the failure is terminal.
const createTreeMaxAttempts = 3

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
	var err error
	rbacName := fmt.Sprintf(RBACNameMask, i.component)

	labels := labels.For("createtree", i.component, instance.GetName())

	// ServiceAccount
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*corev1.ServiceAccount](instance, i.Client),
		ensure.Labels[*corev1.ServiceAccount](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create SA: %w", err), instance)
	}

	// Role
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*rbacv1.Role](instance, i.Client),
		ensure.Labels[*rbacv1.Role](slices.Collect(maps.Keys(labels)), labels),
		kubernetes.EnsureRoleRules(
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"patch"},
			}),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Role: %w", err), instance)
	}

	// RoleBinding
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbacName,
			Namespace: instance.GetNamespace(),
		},
	},
		ensure.ControllerReference[*rbacv1.RoleBinding](instance, i.Client),
		ensure.Labels[*rbacv1.RoleBinding](slices.Collect(maps.Keys(labels)), labels),
		kubernetes.EnsureRoleBinding(
			rbacv1.RoleRef{
				APIGroup: corev1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     rbacName,
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: rbacName, Namespace: instance.GetNamespace()},
		),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create RoleBinding: %w", err), instance)
	}

	return i.Continue()
}

func (i resolveTree[T]) handleConfigMap(ctx context.Context, instance T) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For("createtree", i.component, instance.GetName())

	// Needed for configMap clean-up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(configMapResultMask, i.component, instance.GetName()),
			Namespace: instance.GetNamespace(),
		},
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		configMap,
		ensure.ControllerReference[*corev1.ConfigMap](instance, i.Client),
		ensure.Labels[*corev1.ConfigMap](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create %s ConfigMap: %w", configMap.GetName(), err), instance)
	}

	if result != controllerutil.OperationResultNone {
		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: fmt.Sprintf("ConfigMap `%s` %s", configMap.GetName(), result)},
		)
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i resolveTree[T]) handleJob(ctx context.Context, instance T) *action.Result {
	var err error
	var trillUrl string
	wrapped := i.wrapper(instance)

	labels := labels.For("createtree", i.component, instance.GetName())

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.RequeueAfter(5 * time.Second)
		}
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	for _, ref := range configMap.GetOwnerReferences() {
		if ref.Kind == "Job" { //nolint:goconst
			return i.Continue()
		}
	}

	// Wait out the retry backoff before recreating a failed Job. A generation
	// change (spec edit) resets attempts and skips the gate.
	if readAttempts(configMap, instance.GetGeneration()) > 0 {
		if nextRetry, ok := readNextRetry(configMap); ok {
			if remaining := time.Until(nextRetry); remaining > 0 {
				i.Logger.V(1).Info("waiting before recreating failed createtree job", "remaining", remaining.String())
				return i.RequeueAfter(remaining)
			}
		}
	}

	trillianService := wrapped.GetTrillianService()

	trillHost, trillPort, err := serviceresolver.ResolveInternalGrpcService(ctx, i.Client, *trillianService, instance.GetNamespace(), &rhtasv1.Trillian{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve trillian service: %w", err), instance)
	}
	trillUrl = fmt.Sprintf("%s:%s", trillHost, trillPort)
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(JobNameMask, i.component),
			Namespace:    instance.GetNamespace(),
		},
	}

	extraArgs := []string{}
	if instance.GetTrustedCA() != nil || kubernetes.IsOpenShift() {
		caPath, err := tls.CAPath(ctx, i.Client, instance)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("could not get CA path: %w", err), instance)
		}
		extraArgs = append(extraArgs, "--tls_cert_file", caPath)
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		job,
		i.ensureJob(fmt.Sprintf(configMapResultMask, i.component, instance.GetName()), trillUrl, i.treeDisplayName, extraArgs...),
		ensure.ControllerReference[*batchv1.Job](instance, i.Client),
		ensure.Labels[*batchv1.Job](slices.Collect(maps.Keys(labels)), labels),
		func(object *batchv1.Job) error {
			object.Spec.Template.Labels = labels
			return nil
		},
		func(object *batchv1.Job) error {
			return ensure.PodSecurityContext(&object.Spec.Template.Spec)
		},
		func(object *batchv1.Job) error {
			return ensure.GODEBUG(instance.GetAnnotations())(&object.Spec.Template.Spec)
		},
		func(object *batchv1.Job) error {
			return ensureTls.TrustedCA(instance.GetTrustedCA(), createTreeContainerName)(&object.Spec.Template)
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create segment backup job: %w", err), instance,
			metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Creating.String(),
				Message: err.Error(),
			})
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client, configMap,
		func(object *corev1.ConfigMap) error {
			return controllerutil.SetOwnerReference(job, object, i.Client.Scheme())
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not update annotations on %s ConfigMap: %w", configMap.GetName(), err), instance,
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

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
	configMap, err := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), configMapName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return i.RequeueAfter(5 * time.Second)
		}
		return i.Error(ctx, fmt.Errorf("could not get configmap: %w", err), instance)
	}

	for _, ref := range configMap.GetOwnerReferences() {
		if ref.Kind == "Job" {
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
		// Tree was already created and written: adopt it instead of orphaning it.
		if result, ok := configMap.Data[configMapResultField]; ok && result != "" {
			i.Logger.V(1).Info("createtree job failed but result is present; adopting existing tree")
			return i.Continue()
		}

		attempts := readAttempts(configMap, instance.GetGeneration()) + 1

		if attempts >= createTreeMaxAttempts {
			// Keep the failed Job for debugging and go terminal so alerting fires.
			if err = i.recordAttempts(ctx, configMap, instance.GetGeneration(), attempts); err != nil {
				return i.Error(ctx, fmt.Errorf("could not record createtree attempts: %w", err), instance)
			}
			i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CreateTreeJobFailed", "GaveUp",
				"createtree job %s failed %d times; giving up. Fix the underlying cause and edit the resource to retry", j.GetName(), attempts)
			return i.Error(ctx, reconcile.TerminalError(ErrJobFailed), instance, metav1.Condition{
				Type:    JobCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: ErrJobFailed.Error(),
			})
		}

		// Delete the failed Job, drop its owner reference so handleJob recreates
		// it, and requeue with linear backoff.
		backoff := createTreeRetryBaseDelay * time.Duration(attempts)
		nextRetry := time.Now().Add(backoff)
		if err = i.cleanupFailedJob(ctx, j, configMap, instance.GetGeneration(), attempts, nextRetry); err != nil {
			return i.Error(ctx, fmt.Errorf("could not clean up failed createtree job: %w", err), instance)
		}

		i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CreateTreeJobFailed", "Retrying",
			"createtree job %s failed; retrying tree initialization (attempt %d/%d)", j.GetName(), attempts, createTreeMaxAttempts)

		instance.SetCondition(metav1.Condition{
			Type:    JobCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: fmt.Sprintf("createtree job failed; retrying tree initialization (attempt %d/%d)", attempts, createTreeMaxAttempts),
		})
		if _, err = i.PersistStatus(ctx, instance); err != nil {
			return i.Error(ctx, err, instance)
		}
		return i.RequeueAfter(backoff)
	}

	return i.Continue()
}

// readAttempts returns the failed-attempt count recorded for the given generation.
func readAttempts(configMap *corev1.ConfigMap, generation int64) int {
	anns := configMap.GetAnnotations()
	if anns == nil {
		return 0
	}
	if anns[annotations.CreateTreeAttemptsGeneration] != strconv.FormatInt(generation, 10) {
		return 0
	}
	attempts, err := strconv.Atoi(anns[annotations.CreateTreeAttempts])
	if err != nil {
		return 0
	}
	return attempts
}

// readNextRetry returns the scheduled next-retry time recorded on the ConfigMap.
func readNextRetry(configMap *corev1.ConfigMap) (time.Time, bool) {
	anns := configMap.GetAnnotations()
	if anns == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, anns[annotations.CreateTreeNextRetry])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// recordAttempts persists the attempt count and generation on the ConfigMap.
func (i resolveTree[T]) recordAttempts(ctx context.Context, configMap *corev1.ConfigMap, generation int64, attempts int) error {
	if _, err := kubernetes.CreateOrUpdate(ctx, i.Client, configMap,
		setAttemptsAnnotations(generation, attempts),
	); err != nil {
		return fmt.Errorf("could not update %s ConfigMap: %w", configMap.GetName(), err)
	}
	return nil
}

// setAttemptsAnnotations records the attempt count and generation annotations.
func setAttemptsAnnotations(generation int64, attempts int) func(*corev1.ConfigMap) error {
	return func(object *corev1.ConfigMap) error {
		anns := object.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		anns[annotations.CreateTreeAttempts] = strconv.Itoa(attempts)
		anns[annotations.CreateTreeAttemptsGeneration] = strconv.FormatInt(generation, 10)
		object.SetAnnotations(anns)
		return nil
	}
}

// setNextRetryAnnotation records the earliest time the Job may be recreated.
func setNextRetryAnnotation(nextRetry time.Time) func(*corev1.ConfigMap) error {
	return func(object *corev1.ConfigMap) error {
		anns := object.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		anns[annotations.CreateTreeNextRetry] = nextRetry.UTC().Format(time.RFC3339)
		object.SetAnnotations(anns)
		return nil
	}
}

// clearRetryAnnotations removes the retry bookkeeping annotations.
func clearRetryAnnotations(object *corev1.ConfigMap) error {
	anns := object.GetAnnotations()
	if anns == nil {
		return nil
	}
	delete(anns, annotations.CreateTreeAttempts)
	delete(anns, annotations.CreateTreeAttemptsGeneration)
	delete(anns, annotations.CreateTreeNextRetry)
	object.SetAnnotations(anns)
	return nil
}

// cleanupFailedJob drops the Job owner reference (so handleJob recreates the
// Job), records the attempt count and next-retry time, and deletes the Job.
func (i resolveTree[T]) cleanupFailedJob(ctx context.Context, j *batchv1.Job, configMap *corev1.ConfigMap, generation int64, attempts int, nextRetry time.Time) error {
	if _, err := kubernetes.CreateOrUpdate(ctx, i.Client, configMap,
		func(object *corev1.ConfigMap) error {
			refs := object.GetOwnerReferences()
			filtered := make([]metav1.OwnerReference, 0, len(refs))
			for _, ref := range refs {
				if ref.Kind != "Job" { //nolint:goconst
					filtered = append(filtered, ref)
				}
			}
			object.SetOwnerReferences(filtered)
			return nil
		},
		setAttemptsAnnotations(generation, attempts),
		setNextRetryAnnotation(nextRetry),
	); err != nil {
		return fmt.Errorf("could not remove Job owner reference from %s ConfigMap: %w", configMap.GetName(), err)
	}

	propagation := metav1.DeletePropagationBackground
	if err := i.Client.Delete(ctx, j, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("could not delete failed Job %s: %w", j.GetName(), err)
	}
	return nil
}

func (i resolveTree[T]) handleExtractJobResult(ctx context.Context, instance T) *action.Result {
	wrapped := i.wrapper(instance)

	configMapName := fmt.Sprintf(configMapResultMask, i.component, instance.GetName())
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

		// Tree resolved: clear any retry bookkeeping left by earlier failures.
		if _, err := kubernetes.CreateOrUpdate(ctx, i.Client, configMap, clearRetryAnnotations); err != nil {
			return i.Error(ctx, fmt.Errorf("could not clear retry annotations on %s ConfigMap: %w", configMap.GetName(), err), instance)
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

func (i resolveTree[T]) ensureJob(cfgName, adminServer, displayName string, extraArgs ...string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		spec := &job.Spec
		spec.Parallelism = utils.Pointer[int32](1)
		spec.Completions = utils.Pointer[int32](1)
		spec.ActiveDeadlineSeconds = utils.Pointer[int64](600)
		spec.BackoffLimit = utils.Pointer[int32](5)

		templateSpec := &spec.Template.Spec
		templateSpec.ServiceAccountName = fmt.Sprintf(RBACNameMask, i.component)
		templateSpec.RestartPolicy = "OnFailure"

		container := kubernetes.FindContainerByNameOrCreate(templateSpec, createTreeContainerName)
		container.Image = images.Registry.Get(images.TrillianCreateTree)
		container.Command = []string{"/bin/sh", "-c"}
		container.Args = []string{string(jobScript)}

		cfgNameEnv := kubernetes.FindEnvByNameOrCreate(container, "CONFIGMAP_NAME")
		cfgNameEnv.Value = cfgName

		adminServerEnv := kubernetes.FindEnvByNameOrCreate(container, "ADMIN_SERVER")
		adminServerEnv.Value = adminServer

		displayNameEnv := kubernetes.FindEnvByNameOrCreate(container, "DISPLAY_NAME")
		displayNameEnv.Value = displayName

		extraArgsEnv := kubernetes.FindEnvByNameOrCreate(container, "EXTRA_ARGS")
		extraArgsEnv.Value = strings.Join(extraArgs, " ")

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
