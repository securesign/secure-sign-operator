package rekor

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	utils2 "github.com/securesign/operator/controllers/common/utils"
	"github.com/securesign/operator/controllers/rekor/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const rekorDeploymentName = "rekor-server"

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(Rekor *rhtasv1alpha1.Rekor) bool {
	return Rekor.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) (*rhtasv1alpha1.Rekor, error) {
	//log := ctrllog.FromContext(ctx)
	var err error
	if instance.Spec.KeySecret == "" {
		instance.Spec.KeySecret = "rekor-private-key"
	}

	if instance.Spec.RekorCert.Create {

		certConfig, err := utils.CreateRekorKey()
		if err != nil {
			return instance, err
		}

		secret := utils2.CreateSecret(instance.Namespace, instance.Spec.KeySecret, "rekor-server", "rekor", map[string]string{"private": certConfig.RekorKey})
		controllerutil.SetOwnerReference(instance, secret, i.Client.Scheme())
		if err = i.Client.Create(ctx, secret); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create rekor secret: %w", err)
		}
	}

	sharding := i.initConfigmap(instance.Namespace, "rekor-sharding-config")
	if err = i.Client.Create(ctx, sharding); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor secret: %w", err)
	}

	var rekorPvcName string
	if instance.Spec.PvcName == "" {
		rekorPvc := utils2.CreatePVC(instance.Namespace, "rekor-server", "5Gi")
		if err = i.Client.Create(ctx, rekorPvc); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create Rekor secret: %w", err)
		}
		rekorPvcName = rekorPvc.Name
		// TODO: add status field
	} else {
		rekorPvcName = instance.Spec.PvcName
	}

	config := i.initConfigmap(instance.Namespace, "rekor-config")
	if err = i.Client.Create(ctx, config); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor secret: %w", err)
	}

	dp := utils.CreateRekorDeployment(instance.Namespace, rekorDeploymentName, rekorPvcName)
	controllerutil.SetControllerReference(instance, dp, i.Client.Scheme())
	if err = i.Client.Create(ctx, dp); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor deployment: %w", err)
	}

	redis := utils.CreateRedisDeployment(instance.Namespace, "rekor-redis")
	controllerutil.SetControllerReference(instance, redis, i.Client.Scheme())
	if err = i.Client.Create(ctx, redis); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor-redis deployment: %w", err)
	}

	svc := utils2.CreateService(instance.Namespace, rekorDeploymentName, rekorDeploymentName, rekorDeploymentName, 2112)
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

	// TODO: move code from job to operator
	tree := utils.CTJob(instance.Namespace, "create-tree-rekor")
	if err = i.Client.Create(ctx, tree); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	if instance.Spec.External {
		// TODO: do we need to support ingress?
		route := utils2.CreateRoute(*svc, "80-tcp")
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
func (i initializeAction) initConfigmap(namespace string, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "rekor",
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},

		Data: map[string]string{
			"sharding-config.yaml": ""},
	}
}
