package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"gomodules.xyz/jsonpatch/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/utils/ptr"
)

const (
	syncJobName                     = "trust-root-sync"
	trustRootDesiredAnnotation      = "rhtas.redhat.com/trust-root"
	tufObservedGenerationAnnotation = "rhtas.redhat.com/tuf-observed-generation"
	trustRootPatchAnnotation        = "rhtas.redhat.com/trust-root-patch"
	repositoryMountPath             = "/var/run/repository"
)

func NewDetectSyncDriftAction() action.Action[*rhtasv1.Tuf] {
	return &detectSyncDriftAction{}
}

type detectSyncDriftAction struct {
	action.BaseAction
}

func (i detectSyncDriftAction) Name() string {
	return "synchronize trust root repository"
}

func (i detectSyncDriftAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	return state.FromInstance(instance, constants.ReadyCondition) == state.Ready &&
		instance.Annotations[tufConstants.RepositoryVersionAnnotation] == tufConstants.TufVersionV1
}

func (i detectSyncDriftAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	specComponents := rhtasv1.Components{
		Ctlog:  instance.Spec.Ctlog,
		Fulcio: instance.Spec.Fulcio,
		Rekor:  instance.Spec.Rekor,
		Tsa:    ptr.Deref(instance.Spec.Tsa, nil),
	}
	desired, err := trustroot.Resolve(ctx, i.Client, instance.Namespace, specComponents)
	if err != nil {
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:    tufConstants.TrustRootSynced,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	actual, err := trustroot.Resolve(ctx, i.Client, instance.Namespace, ptr.Deref(instance.Status.TrustRoot.Published, rhtasv1.Components{}))
	if err != nil {
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:    tufConstants.TrustRootSynced,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if reflect.DeepEqual(desired, actual) {
		return i.Continue()
	}

	// generate the diff
	patch, err := createJSONPatch(actual, desired, fmt.Sprintf("rhtas.tufcli/%s", instance.Annotations[tufConstants.RepositoryVersionAnnotation]))
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	specComponentsAnnotation, err := json.Marshal(specComponents)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	job, err := traits.Apply(
		batchv1ac.Job(syncJobName, instance.Namespace),
		traits.Annotations[*batchv1ac.JobApplyConfiguration](map[string]string{
			tufObservedGenerationAnnotation: strconv.FormatInt(instance.Generation, 10),
			trustRootDesiredAnnotation:      string(specComponentsAnnotation),
			trustRootPatchAnnotation:        string(patch),
		}),
		traits.Labels[*batchv1ac.JobApplyConfiguration](labels.ForComponent(tufConstants.ComponentName, instance.Name)),
		ensureSyncJob(instance),
		traits.WithJobPodSpecTraits(
			traits.Proxy([]string{utils.TufInitContainerName}),
			traits.GODEBUG(instance.GetAnnotations(), utils.TufInitContainerName),
			traits.SecurityContexts(utils.TufInitContainerName),
			traits.TrustedCA(instance.GetTrustedCA(), utils.TufInitContainerName),
			traits.ServiceAccount(tufConstants.RBACInitJobName),
		),
	)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	if err := i.Client.Apply(ctx, job); err != nil {
		return i.Error(ctx, err, instance)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               tufConstants.TrustRootSynced,
		Status:             metav1.ConditionFalse,
		Reason:             tufConstants.SyncInProgress,
		Message:            fmt.Sprintf("TUF trust root synchronization job in progress: %s", syncJobName), // add job-name ref
		ObservedGeneration: instance.Generation,
	})

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func createJSONPatch(actual, desired trustroot.ResolvedTrustRootDocument, apiVersion string) ([]byte, error) {
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return nil, err
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(actualJSON, desiredJSON)
	if err != nil {
		return nil, err
	}

	patch = slices.Insert(patch, 0, jsonpatch.NewOperation("test", "/apiVersion", apiVersion))
	return json.Marshal(patch)
}

func ensureSyncJob(instance *rhtasv1.Tuf) traits.Trait[*batchv1ac.JobApplyConfiguration] {
	return func(job *batchv1ac.JobApplyConfiguration) error {
		const (
			keyVolumeName, keyVolumeMountPath                                      = "keys", "/var/run/keys"
			trustRootPatchVolumeName, trustRootPatchVolumeMountPath, patchFileName = "trust-root-patch", "/var/run/trust-root-patch", "patch.json"
		)
		keys := []string{"targets.pem", "timestamp.pem", "snapshot.pem"}

		container := corev1ac.Container().
			WithName(syncJobName).
			WithImage(images.Registry.Get(images.Tuf)).
			WithCommand("tufcli", "rhtas", "apply").
			WithArgs("--root", repositoryMountPath+"/root.json").
			WithArgs("--operator", tufConstants.OperatorName).
			WithArgs("--outdir", repositoryMountPath).
			WithArgs("--input", trustRootPatchVolumeMountPath+"/"+patchFileName).
			WithVolumeMounts(
				corev1ac.VolumeMount().WithName(tufConstants.VolumeName).WithMountPath(repositoryMountPath).WithReadOnly(false),
				corev1ac.VolumeMount().WithName(keyVolumeName).WithMountPath(keyVolumeMountPath).WithReadOnly(false),
				corev1ac.VolumeMount().WithName(trustRootPatchVolumeName).WithMountPath(trustRootPatchVolumeMountPath),
			)

		keySecret := corev1ac.SecretVolumeSource().WithSecretName(instance.Spec.RootKeySecretRef.Name)
		for _, key := range keys {
			container.WithArgs("--key", keyVolumeMountPath+"/"+key)
			keySecret.WithItems(corev1ac.KeyToPath().WithKey(key).WithPath(key))
		}

		podSpec := corev1ac.PodSpec().
			WithRestartPolicy(v1.RestartPolicyNever).
			WithContainers(container).
			WithVolumes(
				corev1ac.Volume().WithName(tufConstants.VolumeName).
					WithPersistentVolumeClaim(corev1ac.PersistentVolumeClaimVolumeSource().WithClaimName(instance.Status.PvcName)),
				corev1ac.Volume().WithName(keyVolumeName).WithSecret(keySecret),
				corev1ac.Volume().WithName(trustRootPatchVolumeName).WithDownwardAPI(
					corev1ac.DownwardAPIVolumeSource().WithItems(
						corev1ac.DownwardAPIVolumeFile().
							WithPath(patchFileName).
							WithFieldRef(corev1ac.ObjectFieldSelector().WithFieldPath(fmt.Sprintf("metadata.annotations['%s']", trustRootPatchAnnotation))),
					),
				),
			)

		job.WithSpec(batchv1ac.JobSpec().
			WithParallelism(1).
			WithCompletions(1).
			WithBackoffLimit(3).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(job.Labels).WithAnnotations(job.Annotations).WithSpec(podSpec)))

		return nil
	}
}
