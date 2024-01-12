package fulcio

import (
	"context"
	"encoding/json"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/fulcio/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	fulcioDeploymentName = "fulcio-server"
	ComponentName        = "fulcio"
)

func NewCreateAction() Action {
	return &createAction{}
}

type createAction struct {
	common.BaseAction
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
