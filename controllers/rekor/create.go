package rekor

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/rekor/utils"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	RekorDeploymentName         = "rekor-server"
	rekorRedisDeploymentName    = "rekor-redis"
	RekorSearchUiDeploymentName = "rekor-search-ui"
	ComponentName               = "rekor"
	rekorMonitoringRoleName     = "prometheus-k8s-rekor"
	rekorServiceMonitorName     = "rekor-metrics"
	RekorServiceAccountName     = "rekor-sa"
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
	rekorServerLabels[k8sutils.NameLabel] = RekorDeploymentName

	sharding := k8sutils.InitConfigmap(instance.Namespace, "rekor-sharding-config", rekorServerLabels, map[string]string{"sharding-config.yaml": ""})
	controllerutil.SetControllerReference(instance, sharding, i.Client.Scheme())
	if err = i.Client.Create(ctx, sharding); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor secret: %w", err)
	}

	if instance.Spec.PvcName == "" {
		// Check if the PVC already exists
		exists, err := k8sutils.GetPVC(ctx, i.Client, instance.Namespace, "rekor-server")
		if err != nil {
			// Error while checking for PVC existence
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not check for existing Rekor PVC: %w", err)
		}
		if exists {
			// PVC already exists, use its name
			i.Logger.V(1).Info("PVC already exists reusing")
			instance.Spec.PvcName = "rekor-server"
		} else {
			// PVC does not exist, create a new one
			i.Logger.V(1).Info("Creating new PVC")
			rekorPvc := k8sutils.CreatePVC(instance.Namespace, "rekor-server", "5Gi")
			if err := i.Client.Create(ctx, rekorPvc); err != nil {
				instance.Status.Phase = rhtasv1alpha1.PhaseError
				return instance, fmt.Errorf("could not create Rekor PVC: %w", err)
			}
			instance.Spec.PvcName = rekorPvc.Name
		}
		// TODO: add status field
	}

	trillian, err := trillianUtils.FindTrillian(ctx, i.Client, instance.Namespace, k8sutils.FilterCommonLabels(instance.Labels))
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not find trillian TreeID: %w", err)
	}

	if instance.Spec.TreeID == nil || *instance.Spec.TreeID == int64(0) {
		tree, err := common.CreateTrillianTree(ctx, "ctlog-tree", trillian.Status.Url)
		if err != nil {
			return instance, fmt.Errorf("could not create ctlog-tree: %w", err)
		}
		instance.Status.TreeID = &tree.TreeId
	} else {
		instance.Status.TreeID = instance.Spec.TreeID
	}

	sa := kubernetes.CreateServiceAccount(instance.Namespace, RekorServiceAccountName, rekorServerLabels)
	controllerutil.SetControllerReference(instance, sa, i.Client.Scheme())
	if err = i.Client.Create(ctx, sa); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create rekor sa: %w", err)
	}

	dp := utils.CreateRekorDeployment(instance, RekorDeploymentName, rekorServerLabels, sa.Name)
	controllerutil.SetControllerReference(instance, dp, i.Client.Scheme())
	if err = i.Client.Create(ctx, dp); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor deployment: %w", err)
	}

	redis := utils.CreateRedisDeployment(instance.Namespace, "rekor-redis", redisLabels, RekorServiceAccountName)
	controllerutil.SetControllerReference(instance, redis, i.Client.Scheme())
	if err = i.Client.Create(ctx, redis); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor-redis deployment: %w", err)
	}

	redisService := k8sutils.CreateService(instance.Namespace, rekorRedisDeploymentName, 6379, redisLabels)
	controllerutil.SetControllerReference(instance, redisService, i.Client.Scheme())
	if err = i.Client.Create(ctx, redisService); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create redis service: %w", err)
	}

	svc := k8sutils.CreateService(instance.Namespace, RekorDeploymentName, 2112, rekorServerLabels)
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

	if instance.Spec.RekorSearchUI.Enabled {
		rekorSearchUiLabels := k8sutils.FilterCommonLabels(instance.Labels)
		rekorSearchUiLabels[k8sutils.ComponentLabel] = ComponentName
		rekorSearchUiLabels[k8sutils.NameLabel] = RekorSearchUiDeploymentName

		rekorSearchUi := utils.CreateRekorSearchUiDeployment(instance.Namespace, RekorSearchUiDeploymentName, rekorSearchUiLabels)
		controllerutil.SetControllerReference(instance, rekorSearchUi, i.Client.Scheme())
		if err = i.Client.Create(ctx, rekorSearchUi); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create Rekor-Search-UI deployment: %w", err)
		}

		rekorSearchUiService := k8sutils.CreateService(instance.Namespace, RekorSearchUiDeploymentName, 3000, rekorSearchUiLabels)
		controllerutil.SetControllerReference(instance, rekorSearchUiService, i.Client.Scheme())
		if err = i.Client.Create(ctx, rekorSearchUiService); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create Rekor-Search-UI service: %w", err)
		}

		rekorSearchUiIngress, err := k8sutils.CreateIngress(ctx, i.Client, *rekorSearchUiService, rhtasv1alpha1.ExternalAccess{}, RekorSearchUiDeploymentName, rekorSearchUiLabels)
		controllerutil.SetControllerReference(instance, rekorSearchUiIngress, i.Client.Scheme())
		if err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create ingress: %w", err)
		}
		controllerutil.SetControllerReference(instance, rekorSearchUiIngress, i.Client.Scheme())
		if err = i.Client.Create(ctx, rekorSearchUiIngress); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create route: %w", err)
		}
		instance.Status.RekorSearchUIPhase = rhtasv1alpha1.PhaseInitialize
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseInitialize
	return instance, nil

}
