package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	utils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	ctlogUtils "github.com/securesign/operator/controllers/ctlog/utils"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const ConfigSecretNameFormat = "ctlog-%s-config"

func NewServerConfigAction() action.Action[rhtasv1alpha1.CTlog] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

func (i serverConfig) CanHandle(instance *rhtasv1alpha1.CTlog) bool {
	return instance.Status.Phase == rhtasv1alpha1.PhaseCreating || instance.Status.Phase == rhtasv1alpha1.PhaseReady
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err     error
		updated bool
	)
	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	trillUrl, err := utils.GetInternalUrl(ctx, i.Client, instance.Namespace, trillian.LogserverDeploymentName)
	if err != nil {
		return i.Failed(err)
	}

	rootCerts, err := i.handleRootCertificates(instance)
	if err != nil {
		return i.Failed(err)
	}

	certConfig, err := i.handlePrivateKey(instance)
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	var config *corev1.Secret
	//TODO: the config is generated in every reconcile loop rotation - it can cause performance issues
	if config, err = ctlogUtils.CreateCtlogConfig(ctx, instance.Namespace, trillUrl+":8091", *instance.Spec.TreeID, rootCerts, labels, certConfig); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create CTLog configuration: %w", err), instance)
	}
	// patch secret name
	config.Name = fmt.Sprintf(ConfigSecretNameFormat, instance.Name)

	if err = controllerutil.SetControllerReference(instance, config, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if _, err = i.Ensure(ctx, config); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Secret: %w", err), instance)
	}

	if updated {
		return i.Requeue()
	} else {
		return i.Continue()
	}

}

func (i serverConfig) handlePrivateKey(instance *rhtasv1alpha1.CTlog) (*ctlogUtils.PrivateKeyConfig, error) {
	private, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Spec.PrivateKeyRef)
	if err != nil {
		return nil, err
	}
	public, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Spec.PublicKeyRef)
	if err != nil {
		return nil, err
	}
	password, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Spec.PrivateKeyPasswordRef)
	if err != nil {
		return nil, err
	}

	return &ctlogUtils.PrivateKeyConfig{
		PrivateKey:     private,
		PublicKey:      public,
		PrivateKeyPass: password,
	}, nil
}

func (i serverConfig) handleRootCertificates(instance *rhtasv1alpha1.CTlog) ([]ctlogUtils.RootCertificate, error) {
	certs := make([]ctlogUtils.RootCertificate, 0)

	for _, selector := range instance.Spec.RootCertificates {
		data, err := utils.GetSecretData(i.Client, instance.Namespace, &selector)
		if err != nil {
			return nil, err
		}
		certs = append(certs, data)
	}

	return certs, nil
}
