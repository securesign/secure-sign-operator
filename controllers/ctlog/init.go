package ctlog

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils"
	ctlogUtils "github.com/securesign/operator/controllers/ctlog/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	deploymentName = "ctlog"
	jobName        = "ctlog-createtree"
)

func NewInitializeAction() Action {
	return &initializeAction{}
}

type initializeAction struct {
	common.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(ctlog *rhtasv1alpha1.CTlog) bool {
	return ctlog.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) (*rhtasv1alpha1.CTlog, error) {
	//log := ctrllog.FromContext(ctx)
	var err error

	server := ctlogUtils.CreateDeployment(instance.Namespace, deploymentName, "ctlog")
	controllerutil.SetControllerReference(instance, server, i.Client.Scheme())
	if err = i.Client.Create(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	cm := i.initConfigmap(instance.Namespace, "ctlog-config")
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

	// TODO: move code from job to operator
	tree := ctlogUtils.CTJob(instance.Namespace, "create-tree")
	if err = i.Client.Create(ctx, tree); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	svc := utils.CreateService(instance.Namespace, "ctlog", "ctlog", "ctlog", 6963)
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

	instance.Status.Phase = rhtasv1alpha1.PhaseInitialization
	return instance, nil

}

func (i initializeAction) initConfigmap(namespace string, name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "ctlog",
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},

		Data: map[string]string{
			"__placeholder": "###################################################################\n" +
				"# Just a placeholder so that reapplying this won't overwrite treeID\n" +
				"# if it already exists. This caused grief, do not remove.\n" +
				"###################################################################",
		},
	}
}
