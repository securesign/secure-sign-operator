package rekor

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/rekor/utils"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	rekorDeploymentName      = "rekor-server"
	rekorRedisDeploymentName = "rekor-redis"
	ComponentName            = "rekor"
	rekorMonitoringRoleName  = "prometheus-k8s-rekor"
	rekorServiceMonitorName  = "rekor-metrics"
)

func NewCreateAction() action.Action[rhtasv1alpha1.Rekor] {
	return &createAction{}
}

type createAction struct {
	action.BaseAction
}

func (i createAction) Name() string {
	return "create"
}

func (i createAction) CanHandle(Rekor *rhtasv1alpha1.Rekor) bool {
	return Rekor.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
	//log := ctrllog.FromContext(ctx)
	var err error

	redisLabels := k8sutils.FilterCommonLabels(instance.Labels)
	redisLabels[k8sutils.ComponentLabel] = ComponentName
	redisLabels[k8sutils.NameLabel] = rekorRedisDeploymentName

	rekorServerLabels := k8sutils.FilterCommonLabels(instance.Labels)
	rekorServerLabels[k8sutils.ComponentLabel] = ComponentName
	rekorServerLabels[k8sutils.NameLabel] = rekorDeploymentName

	if instance.Spec.Certificate.Create {
		certConfig, err := utils.CreateRekorKey()
		if err != nil {
			return instance, err
		}

		secret := k8sutils.CreateSecret(instance.Spec.Certificate.SecretName, instance.Namespace, map[string][]byte{"private": certConfig.RekorKey}, rekorServerLabels)
		controllerutil.SetOwnerReference(instance, secret, i.Client.Scheme())
		if err = i.Client.Create(ctx, secret); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create rekor secret: %w", err)
		}
	}

	sharding := k8sutils.InitConfigmap(instance.Namespace, "rekor-sharding-config", rekorServerLabels, map[string]string{"sharding-config.yaml": ""})
	controllerutil.SetControllerReference(instance, sharding, i.Client.Scheme())
	if err = i.Client.Create(ctx, sharding); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor secret: %w", err)
	}

	var rekorPvcName string
	if instance.Spec.PvcName == "" {
		rekorPvc := k8sutils.CreatePVC(instance.Namespace, "rekor-server", "5Gi")
		if err = i.Client.Create(ctx, rekorPvc); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create Rekor PVC: %w", err)
		}
		rekorPvcName = rekorPvc.Name
		// TODO: add status field
	} else {
		rekorPvcName = instance.Spec.PvcName
	}

	trillian, err := trillianUtils.FindTrillian(ctx, i.Client, instance.Namespace, k8sutils.FilterCommonLabels(instance.Labels))
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not find trillian TreeID: %w", err)
	}

	dp := utils.CreateRekorDeployment(instance.Namespace, rekorDeploymentName, trillian.Status.TreeID, rekorPvcName, instance.Spec.Certificate.SecretName, rekorServerLabels)
	controllerutil.SetControllerReference(instance, dp, i.Client.Scheme())
	if err = i.Client.Create(ctx, dp); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor deployment: %w", err)
	}

	redis := utils.CreateRedisDeployment(instance.Namespace, "rekor-redis", redisLabels)
	controllerutil.SetControllerReference(instance, redis, i.Client.Scheme())
	if err = i.Client.Create(ctx, redis); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor-redis deployment: %w", err)
	}

	svc := k8sutils.CreateService(instance.Namespace, ComponentName, 2112, rekorServerLabels)
	controllerutil.SetControllerReference(instance, svc, i.Client.Scheme())
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "80-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       80,
		TargetPort: intstr.FromInt(3000),
	})
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}

	if instance.Spec.ExternalAccess.Enabled {
		ingress, err := k8sutils.CreateIngress(ctx, i.Client, *svc, instance.Spec.ExternalAccess, "80-tcp", rekorServerLabels)
		if err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create ingress: %w", err)
		}
		controllerutil.SetControllerReference(instance, ingress, i.Client.Scheme())
		if err = i.Client.Create(ctx, ingress); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create route: %w", err)
		}
	}

	if instance.Spec.Monitoring {

		monitoringRoleLabels := k8sutils.FilterCommonLabels(instance.Labels)
		monitoringRoleLabels[k8sutils.ComponentLabel] = ComponentName
		monitoringRoleLabels[k8sutils.NameLabel] = rekorMonitoringRoleName
		role := k8sutils.CreateRole(
			instance.Namespace,
			rekorMonitoringRoleName,
			monitoringRoleLabels,
			[]v1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"services", "endpoints", "pods"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		)
		controllerutil.SetOwnerReference(instance, role, i.Client.Scheme())
		if err = i.Client.Create(ctx, role); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create rekor role: %w", err)
		}

		monitoringRoleBindingLabels := k8sutils.FilterCommonLabels(instance.Labels)
		monitoringRoleBindingLabels[k8sutils.ComponentLabel] = ComponentName
		monitoringRoleBindingLabels[k8sutils.NameLabel] = rekorMonitoringRoleName
		roleBinding := k8sutils.CreateRoleBinding(
			instance.Namespace,
			rekorMonitoringRoleName,
			monitoringRoleBindingLabels,
			v1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     rekorMonitoringRoleName,
			},
			[]v1.Subject{
				{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
			},
		)
		controllerutil.SetOwnerReference(instance, roleBinding, i.Client.Scheme())
		if err = i.Client.Create(ctx, roleBinding); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create rekor roleBinding: %w", err)
		}

		serviceMonitorLabels := k8sutils.FilterCommonLabels(instance.Labels)
		serviceMonitorLabels[k8sutils.ComponentLabel] = ComponentName
		serviceMonitorLabels[k8sutils.NameLabel] = rekorServiceMonitorName

		serviceMonitorMatchLabels := k8sutils.FilterCommonLabels(instance.Labels)
		serviceMonitorMatchLabels[k8sutils.ComponentLabel] = ComponentName

		serviceMonitor := k8sutils.CreateServiceMonitor(
			instance.Namespace,
			rekorServiceMonitorName,
			serviceMonitorLabels,
			[]monitoringv1.Endpoint{
				{
					Interval: monitoringv1.Duration("30s"),
					Port:     "rekor-server",
					Scheme:   "http",
				},
			},
			serviceMonitorMatchLabels,
		)

		if err = i.Client.Create(ctx, serviceMonitor); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create fulcio serviceMonitor: %w", err)
		}
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseInitialize
	return instance, nil

}
