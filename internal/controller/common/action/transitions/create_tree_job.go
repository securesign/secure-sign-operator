package transitions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type TreeJobSupplier[T apis.ConditionsAwareObject] func(
	instance T,
) (
	trillianAddress string,
	instanceName string,
	treeJobConfigMapName string,
	treeJobName string,
	treeDisplayName string,
	trillianDeploymentName string,
	namespace string,
	trillianPort *int32,
	caPath string,
	rbac string,
	labels map[string]string,
	annotations map[string]string,
	treeID *int64,
	err error,
)

func NewCreateTreeJobAction[T apis.ConditionsAwareObject](supplier TreeJobSupplier[T]) action.Action[T] {
	return &createTreeJobAction[T]{supplier: supplier}
}

type createTreeJobAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	supplier TreeJobSupplier[T]
}

func (i createTreeJobAction[T]) Name() string {
	return "create tree job"
}

func (i createTreeJobAction[T]) CanHandle(ctx context.Context, instance T) bool {
	_, _, treeJobConfigMapName, _, _, _, _, _, _, _, _, _, treeID, err := i.supplier(instance)
	if err != nil {
		return false
	}
	cm, _ := kubernetes.GetConfigMap(ctx, i.Client, instance.GetNamespace(), treeJobConfigMapName)
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && cm == nil && treeID == nil
}

func (i createTreeJobAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	trillianAddress, instanceName, treeJobConfigMapName, treeJobName, treeDisplayName, trillianDeploymentName, namespace, trillianPort, caPath, rbac, labels, annotations, _, err := i.supplier(instance)

	if err != nil {
		return i.Failed(fmt.Errorf("failed to get job details: %w", err))
	}

	var trillUrl string

	switch {
	case trillianPort == nil:
		err = fmt.Errorf("%s: %v", i.Name(), utils.TrillianPortNotSpecified)
	case trillianAddress == "":
		trillUrl = fmt.Sprintf("%s.%s.svc:%d", trillianDeploymentName, namespace, *trillianPort)
	default:
		trillUrl = fmt.Sprintf("%s:%d", trillianAddress, *trillianPort)
	}
	if err != nil {
		return i.Failed(err)
	}

	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	instance.SetCondition(metav1.Condition{
		Type:    treeJobName,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Creating tree Job",
	})

	// Needed for configMap clean-up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      treeJobConfigMapName,
			Namespace: instance.GetNamespace(),
			Labels:    labels,
		},
		Data: map[string]string{},
	}

	if err := controllerutil.SetControllerReference(instance, configMap, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for configMap: %w", err))
	}

	updated, err := i.Ensure(ctx, configMap)
	if err != nil {
		instance.SetCondition(metav1.Condition{
			Type:    treeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.Failed(err)
	}
	if updated {
		instance.SetCondition(metav1.Condition{
			Type:    treeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "ConfigMap created",
		})
		i.Recorder.Event(instance, corev1.EventTypeNormal, "TreeConfigCreated", "ConfigMap for TreeID created")
	}

	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)

	trustedCAAnnotation := cutils.TrustedCAAnnotationToReference(annotations)

	cmd := ""
	switch {
	case trustedCAAnnotation != nil:
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s --tls_cert_file=%s", trillUrl, treeDisplayName, caPath)
	case kubernetes.IsOpenShift():
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s --tls_cert_file=/var/run/secrets/tas/tls.crt", trillUrl, treeDisplayName)
	default:
		cmd = fmt.Sprintf("/createtree --admin_server=%s --display_name=%s", trillUrl, treeDisplayName)
	}
	command := []string{
		"/bin/sh",
		"-c",
		fmt.Sprintf(`
		TREE_ID=$(%s)
		if [ $? -eq 0 ]; then
			echo "TREE_ID=$TREE_ID"
			TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
			NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
			API_SERVER=https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}
			curl -k -X PATCH $API_SERVER/api/v1/namespaces/$NAMESPACE/configmaps/"%s" \
				-H "Authorization: Bearer $TOKEN" \
				-H "Content-Type: application/merge-patch+json" \
				-d '{
					"data": {
						"tree_id": "'$TREE_ID'"
					}
				}'
			if [ $? -ne 0 ]; then
				echo "Failed to update ConfigMap" >&2
				exit 1
			fi
		else
			echo "Failed to create tree" >&2
			exit 1
		fi
		`, cmd, treeJobConfigMapName),
	}
	env := []corev1.EnvVar{}

	job := job.CreateJob(namespace, treeJobName, labels, constants.CreateTreeImage, rbac, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err := ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for job: %w", err))
	}

	if err := cutils.SetTrustedCA(&job.Spec.Template, trustedCAAnnotation); err != nil {
		return i.Failed(err)
	}

	if kubernetes.IsOpenShift() && trustedCAAnnotation == nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "tls-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instanceName + "-trillian-server-tls",
				},
			},
		})
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "tls-cert",
			MountPath: "/var/run/secrets/tas",
			ReadOnly:  true,
		})
	}

	if _, err := i.Ensure(ctx, job); err != nil {
		return i.Failed(fmt.Errorf("failed to ensure the job: %w", err))
	}

	instance.SetCondition(metav1.Condition{
		Type:    treeJobName,
		Status:  metav1.ConditionTrue,
		Reason:  constants.Creating,
		Message: "Tree Job created",
	})

	return i.Continue()
}
