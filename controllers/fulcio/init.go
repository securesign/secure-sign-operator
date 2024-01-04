package fulcio

import (
	"context"
	"encoding/json"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	commonUtils "github.com/securesign/operator/controllers/common/utils"
	"github.com/securesign/operator/controllers/fulcio/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const FulcioDeploymentName = "fulcio-server"

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(Fulcio *rhtasv1alpha1.Fulcio) bool {
	return Fulcio.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) (*rhtasv1alpha1.Fulcio, error) {
	//log := ctrllog.FromContext(ctx)
	var err error
	if instance.Spec.KeySecret == "" {
		// TODO: generate one
	}

	cm := i.initConfigmap(instance.Namespace, "fulcio-server-config", *instance)
	controllerutil.SetOwnerReference(instance, cm, i.Client.Scheme())
	if err = i.Client.Create(ctx, cm); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create fulcio secret: %w", err)
	}

	dp := utils.CreateDeployment(instance.Namespace, FulcioDeploymentName, "fulcio-server", "fulcio")
	controllerutil.SetOwnerReference(instance, dp, i.Client.Scheme())
	if err = i.Client.Create(ctx, dp); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create fulcio secret: %w", err)
	}

	svc := commonUtils.CreateService(instance.Namespace, "fulcio-server", "fulcio-server", "fulcio", 2112)
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
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}
	if instance.Spec.External {
		// TODO: do we need to support ingress?
		route := commonUtils.CreateRoute(*svc, "80-tcp")
		if err = i.Client.Create(ctx, route); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create route: %w", err)
		}
		instance.Status.Url = "https://" + route.Spec.Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", svc.Name, svc.Namespace)
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseInitialization
	return instance, nil

}

func (i initializeAction) initConfigmap(namespace string, name string, m rhtasv1alpha1.Fulcio) *corev1.ConfigMap {
	issuers, _ := json.Marshal(m.Spec.OidcIssuers)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "fulcio",
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},

		Data: map[string]string{
			"config.json": fmt.Sprintf("{\"OIDCIssuers\": %s}", issuers),
		},
	}
}
