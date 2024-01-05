package ctlog

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	ctlogUtils "github.com/securesign/operator/controllers/ctlog/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	deploymentName = "ctlog"
	ComponentName  = "ctlog"
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

func (i createAction) CanHandle(ctlog *rhtasv1alpha1.CTlog) bool {
	return ctlog.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) (*rhtasv1alpha1.CTlog, error) {
	//log := ctrllog.FromContext(ctx)
	var err error
	labels := utils.FilterCommonLabels(instance.Labels)
	labels["app.kubernetes.io/component"] = ComponentName
	labels["app.kubernetes.io/name"] = deploymentName

	server := ctlogUtils.CreateDeployment(instance.Namespace, deploymentName, labels)
	controllerutil.SetControllerReference(instance, server, i.Client.Scheme())
	if err = i.Client.Create(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	cm := utils.InitConfigmap(instance.Namespace, "ctlog-config", labels, map[string]string{
		"__placeholder": "###################################################################\n" +
			"# Just a placeholder so that reapplying this won't overwrite treeID\n" +
			"# if it already exists. This caused grief, do not remove.\n" +
			"###################################################################",
	})
	controllerutil.SetControllerReference(instance, cm, i.Client.Scheme())
	if err = i.Client.Create(ctx, cm); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	// TODO: move code from job to operator
	config := ctlogUtils.CreateCTJob(instance.Namespace, "create-config")
	if err = i.Client.Create(ctx, config); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	svc := utils.CreateService(instance.Namespace, "ctlog", 6963, labels)
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       "80-tcp",
		Protocol:   corev1.ProtocolTCP,
		Port:       80,
		TargetPort: intstr.FromInt(6962),
	})
	controllerutil.SetControllerReference(instance, svc, i.Client.Scheme())
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, nil

}
