package actions

import (
	"fmt"

	"context"

	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewSegmentBackupJobAction() action.Action[rhtasv1alpha1.Securesign] {
	return &segmentBackupJob{}
}

type segmentBackupJob struct {
	action.BaseAction
}

func (i segmentBackupJob) Name() string {
	return "segment-backup-installation"
}
func (i segmentBackupJob) CanHandle(_ context.Context, instance *rhtasv1alpha1.Securesign) bool {
	labels := instance.GetLabels()
	for key, value := range labels {
		if key == "rhtas.redhat.com/metrics" && (value == "false" || value == "False") {
			return false
		}
	}
	return true
}

func (i segmentBackupJob) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err error
	)

	labels := constants.LabelsFor(SegmentBackupJobName, SegmentBackupJobName, instance.Name)

	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SegmentRBACName,
			Namespace: instance.Namespace,
			Labels:    labels,
		},
	}

	if err = ctrl.SetControllerReference(instance, sa, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for SA: %w", err))
	}
	// don't re-enqueue for RBAC in any case (except failure)
	_, err = i.Ensure(ctx, sa)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the segment-backup-job service-account: %w", err))
	}

	role := kubernetes.CreateClusterRole(instance.Namespace, SegmentRBACName, labels, []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"update", "get", "list", "patch"},
		},
		{
			APIGroups: []string{"route.openshift.io"},
			Resources: []string{"routes"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"operator.openshift.io"},
			Resources: []string{"consoles"},
			Verbs:     []string{"get", "list"},
		},
	})

	if err = ctrl.SetControllerReference(instance, role, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for role: %w", err))
	}
	_, err = i.Ensure(ctx, role)

	rb := kubernetes.CreateClusterRoleBinding(instance.Namespace, SegmentRBACName, labels, rbacv1.RoleRef{
		APIGroup: v1.SchemeGroupVersion.Group,
		Kind:     "ClusterRole",
		Name:     SegmentRBACName,
	},
		[]rbacv1.Subject{
			{Kind: "ServiceAccount", Name: SegmentRBACName, Namespace: instance.Namespace},
		})

	if err = ctrl.SetControllerReference(instance, rb, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for roleBinding: %w", err))
	}
	_, err = i.Ensure(ctx, rb)

	parallelism := int32(1)
	completions := int32(1)
	activeDeadlineSeconds := int64(600)
	backoffLimit := int32(5)
	command := []string{"python3", "/opt/app-root/src/src/script.py"}
	env := []corev1.EnvVar{
		{
			Name:  "RUN_TYPE",
			Value: "installation",
		},
	}

	job := kubernetes.CreateJob(instance.Namespace, SegmentBackupJobName, labels, constants.SegmentBackupImage, SegmentRBACName, parallelism, completions, activeDeadlineSeconds, backoffLimit, command, env)
	if err = ctrl.SetControllerReference(instance, job, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controll reference for Job: %w", err))
	}
	_, err = i.Ensure(ctx, job)
	if err != nil {
		return i.Failed(fmt.Errorf("failed to Ensure the job: %w", err))
	}
	return i.Continue()
}
