package fulcio

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/securesign/operator/controllers/common/action"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/fulcio/utils"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	fulcioDeploymentName     = "fulcio-server"
	ComponentName            = "fulcio"
	fulcioMonitoringRoleName = "prometheus-k8s-fulcio"
	fulcioServiceMonitorName = "fulcio-metrics"
)

func NewCreateAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &createAction{}
}

type createAction struct {
	action.BaseAction
}

func (i createAction) Name() string {
	return "create"
}

func (i createAction) CanHandle(Fulcio *rhtasv1alpha1.Fulcio) bool {
	return Fulcio.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) (*rhtasv1alpha1.Fulcio, error) {
	//log := ctrllog.FromContext(ctx)
	var err error
	labels := kubernetes.FilterCommonLabels(instance.Labels)
	labels[kubernetes.ComponentLabel] = ComponentName
	labels[kubernetes.NameLabel] = fulcioDeploymentName

	if instance.Spec.Certificate.Create {

		if instance.Spec.Certificate.OrganizationName == "" || instance.Spec.Certificate.OrganizationEmail == "" {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create fulcio cert secret: missing OrganizationName, OrganizationEmail from config")
		}

		certConfig, err := utils.SetupCerts(instance)
		if err != nil {
			return instance, err
		}

		secret := kubernetes.CreateSecret(instance.Spec.Certificate.SecretName, instance.Namespace, map[string][]byte{
			"private":  certConfig.FulcioPrivateKey,
			"public":   certConfig.FulcioPublicKey,
			"cert":     certConfig.FulcioRootCert,
			"password": certConfig.CertPassword,
		}, labels)
		controllerutil.SetOwnerReference(instance, secret, i.Client.Scheme())
		if err = i.Client.Create(ctx, secret); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create fulcio secret: %w", err)
		}
	}

	cm := i.initConfigmap(instance.Namespace, "fulcio-server-config", *instance, labels)
	controllerutil.SetOwnerReference(instance, cm, i.Client.Scheme())
	if err = i.Client.Create(ctx, cm); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create fulcio secret: %w", err)
	}

	dp := utils.CreateDeployment(instance.Namespace, fulcioDeploymentName, instance.Spec.Certificate.SecretName, labels)
	controllerutil.SetControllerReference(instance, dp, i.Client.Scheme())
	if err = i.Client.Create(ctx, dp); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create fulcio secret: %w", err)
	}

	svc := kubernetes.CreateService(instance.Namespace, "fulcio-server", 2112, labels)
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "5554-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       5554,
		TargetPort: intstr.FromInt(5554),
	})
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "80-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       80,
		TargetPort: intstr.FromInt(5555),
	})
	controllerutil.SetControllerReference(instance, svc, i.Client.Scheme())
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}
	if instance.Spec.External {
		// TODO: do we need to support ingress?
		route := kubernetes.CreateRoute(*svc, "80-tcp", labels)
		controllerutil.SetControllerReference(instance, route, i.Client.Scheme())
		if err = i.Client.Create(ctx, route); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create route: %w", err)
		}
		instance.Status.Url = "https://" + route.Spec.Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", svc.Name, svc.Namespace)
	}

	if instance.Spec.Monitoring {
		monitoringRoleLabels := kubernetes.FilterCommonLabels(instance.Labels)
		monitoringRoleLabels[kubernetes.ComponentLabel] = ComponentName
		monitoringRoleLabels[kubernetes.NameLabel] = fulcioMonitoringRoleName
		role := kubernetes.CreateRole(
			instance.Namespace,
			fulcioMonitoringRoleName,
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
			return instance, fmt.Errorf("could not create fulcio role: %w", err)
		}

		monitoringRoleBindingLabels := kubernetes.FilterCommonLabels(instance.Labels)
		monitoringRoleBindingLabels[kubernetes.ComponentLabel] = ComponentName
		monitoringRoleBindingLabels[kubernetes.NameLabel] = fulcioMonitoringRoleName
		roleBinding := kubernetes.CreateRoleBinding(
			instance.Namespace,
			fulcioMonitoringRoleName,
			monitoringRoleBindingLabels,
			v1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     fulcioMonitoringRoleName,
			},
			[]v1.Subject{
				{Kind: "ServiceAccount", Name: "prometheus-k8s", Namespace: "openshift-monitoring"},
			},
		)
		controllerutil.SetOwnerReference(instance, roleBinding, i.Client.Scheme())
		if err = i.Client.Create(ctx, roleBinding); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create fulcio roleBinding: %w", err)
		}

		serviceMonitorLabels := kubernetes.FilterCommonLabels(instance.Labels)
		serviceMonitorLabels[kubernetes.ComponentLabel] = ComponentName
		serviceMonitorLabels[kubernetes.NameLabel] = fulcioServiceMonitorName

		serviceMonitorMatchLabels := kubernetes.FilterCommonLabels(instance.Labels)
		serviceMonitorMatchLabels[kubernetes.ComponentLabel] = ComponentName
		serviceMonitor := kubernetes.CreateServiceMonitor(
			instance.Namespace,
			fulcioDeploymentName,
			serviceMonitorLabels,
			[]monitoringv1.Endpoint{
				{
					Interval: monitoringv1.Duration("30s"),
					Port:     "fulcio-server",
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

func (i createAction) initConfigmap(namespace string, name string, m rhtasv1alpha1.Fulcio, labels map[string]string) *corev1.ConfigMap {
	issuers, _ := json.Marshal(m.Spec.OidcIssuers)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},

		Data: map[string]string{
			"config.json": fmt.Sprintf("{\"OIDCIssuers\": %s}", issuers),
		},
	}
}
