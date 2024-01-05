package rekor

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/rekor/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const rekorDeploymentName = "rekor-server"
const rekorRedisDeploymentName = "rekor-redis"
const ComponentName = "rekor"

func NewCreateAction() Action {
	return &createAction{}
}

type createAction struct {
	common.BaseAction
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
	commonLabels := k8sutils.FilterCommonLabels(instance.Labels)
	commonLabels["app.kubernetes.io/component"] = ComponentName

	redisLabels := commonLabels
	redisLabels["app.kubernetes.io/name"] = rekorRedisDeploymentName

	rekorServerLabels := commonLabels
	rekorServerLabels["app.kubernetes.io/name"] = rekorDeploymentName

	if instance.Spec.KeySecret == "" {
		instance.Spec.KeySecret = "rekor-private-key"
	}

	if instance.Spec.RekorCert.Create {

		certConfig, err := utils.CreateRekorKey()
		if err != nil {
			return instance, err
		}

		secret := k8sutils.CreateSecret(instance.Namespace, instance.Spec.KeySecret, "rekor-server", "rekor", map[string]string{"private": certConfig.RekorKey})
		controllerutil.SetOwnerReference(instance, secret, i.Client.Scheme())
		if err = i.Client.Create(ctx, secret); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create rekor secret: %w", err)
		}
	}

	sharding := k8sutils.InitConfigmap(instance.Namespace, "rekor-sharding-config", rekorServerLabels, map[string]string{"sharding-config.yaml": ""})
	if err = i.Client.Create(ctx, sharding); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create Rekor secret: %w", err)
	}

	var rekorPvcName string
	if instance.Spec.PvcName == "" {
		rekorPvc := k8sutils.CreatePVC(instance.Namespace, "rekor-server", "5Gi")
		if err = i.Client.Create(ctx, rekorPvc); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create Rekor secret: %w", err)
		}
		rekorPvcName = rekorPvc.Name
		// TODO: add status field
	} else {
		rekorPvcName = instance.Spec.PvcName
	}

	dp := utils.CreateRekorDeployment(instance.Namespace, rekorDeploymentName, rekorPvcName, rekorServerLabels)
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

	svc := k8sutils.CreateService(instance.Namespace, rekorDeploymentName, 2112, rekorServerLabels)
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

	if instance.Spec.External {
		// TODO: do we need to support ingress?
		route := k8sutils.CreateRoute(*svc, "80-tcp", rekorServerLabels)
		if err = i.Client.Create(ctx, route); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create route: %w", err)
		}
		instance.Status.Url = "https://" + route.Spec.Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", svc.Name, svc.Namespace)
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, nil

}
