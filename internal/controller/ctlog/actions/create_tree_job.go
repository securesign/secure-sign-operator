package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	cutils "github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	actions2 "github.com/securesign/operator/internal/controller/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCreateTreeJobAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &createTreeJobAction{}
}

type createTreeJobAction struct {
	action.BaseAction
}

func (i createTreeJobAction) Name() string {
	return "create tree job"
}

func (i createTreeJobAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.CTlog) bool {
	cm, _ := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, "ctlog-tree-id-config")
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && cm == nil && instance.Status.TreeID == nil
}

func (i createTreeJobAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err     error
		updated bool
	)

	CtlogTreeJobName := "ctlog-create-tree"
	configMapName := "ctlog-tree-id-config"
	var trillUrl string

	switch {
	case instance.Spec.Trillian.Port == nil:
		err = fmt.Errorf("%s: %v", i.Name(), utils.TrillianPortNotSpecified)
	case instance.Spec.Trillian.Address == "":
		trillUrl = fmt.Sprintf("%s.%s.svc:%d", actions2.LogserverDeploymentName, instance.Namespace, *instance.Spec.Trillian.Port)
	default:
		trillUrl = fmt.Sprintf("%s:%d", instance.Spec.Trillian.Address, *instance.Spec.Trillian.Port)
	}
	if err != nil {
		return i.Failed(err)
	}
	i.Logger.V(1).Info("trillian logserver", "address", trillUrl)

	if c := meta.FindStatusCondition(instance.Status.Conditions, CtlogTreeJobName); c == nil {
		instance.SetCondition(metav1.Condition{
			Type:    CtlogTreeJobName,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Creating ctlog tree Job",
		})
	}

	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	// Needed for configMap clean-up
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{},
	}
	if err = controllerutil.SetControllerReference(instance, configMap, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for configMap: %w", err))
	}
	if updated, err = i.Ensure(ctx, configMap); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
	}
	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "ConfigMap created"})
	}

	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)

	signingKeySecret, _ := kubernetes.GetSecret(i.Client, "openshift-service-ca", "signing-key")
	trustedCAAnnotation := cutils.TrustedCAAnnotationToReference(instance.Annotations)
	cmd := ""
	switch {
	case trustedCAAnnotation != nil:
		cmd = fmt.Sprintf("./createtree --admin_server=%s --display_name=ctlog-tree --tls_cert_file=/var/run/configs/tas/ca-trust/ca-bundle.crt", trillUrl)
	case signingKeySecret != nil:
		cmd = fmt.Sprintf("./createtree --admin_server=%s --display_name=ctlog-tree --tls_cert_file=/etc/ssl/certs/tls.crt", trillUrl)
	default:
		cmd = fmt.Sprintf("./createtree --admin_server=%s --display_name=ctlog-tree", trillUrl)
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
		`, cmd, configMapName),
	}
	env := []corev1.EnvVar{}

	job := kubernetes.CreateJob(instance.Namespace, CtlogTreeJobName, labels, constants.CreateTreeImage, RBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Job: %w", err))
	}

	if trustedCAAnnotation != nil {
		err = cutils.SetTrustedCA(&job.Spec.Template, cutils.TrustedCAAnnotationToReference(instance.Annotations))
		if err != nil {
			return i.Failed(err)
		}
	}

	if signingKeySecret != nil && trustedCAAnnotation == nil {
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "tls-cert",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: instance.Name + "-trillian-log-server-tls-secret",
					},
				},
			})
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "tls-cert",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			})
	}

	_, err = i.Ensure(ctx, job)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the job: %w", err))
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    CtlogTreeJobName,
		Status:  metav1.ConditionTrue,
		Reason:  constants.Ready,
		Message: "ctlog tree Job Created",
	})

	return i.Continue()
}
