package trillian

import (
	"context"
	"fmt"

	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	dbDeploymentName            = "trillian-db"
	logserverDeploymentName     = "trillian-logserver"
	logsignerDeploymentName     = "trillian-logsigner"
	ComponentName               = "trillian"
	dbServiceAccountName        = "trillian-db-sa"
	logsignerServiceAccountName = "trillian-logsigner-sa"
	logserverServiceAccountName = "trillian-logserver-sa"
)

func NewCreateAction() action.Action[rhtasv1alpha1.Trillian] {
	return &createAction{}
}

type createAction struct {
	action.BaseAction
}

func (i createAction) Name() string {
	return "create"
}

func (i createAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	var err error
	dbLabels := k8sutils.FilterCommonLabels(instance.Labels)
	dbLabels[k8sutils.ComponentLabel] = ComponentName
	dbLabels[k8sutils.NameLabel] = dbDeploymentName

	logSignerLabels := k8sutils.FilterCommonLabels(instance.Labels)
	logSignerLabels[k8sutils.ComponentLabel] = ComponentName
	logSignerLabels[k8sutils.NameLabel] = logsignerDeploymentName

	logServerLabels := k8sutils.FilterCommonLabels(instance.Labels)
	logServerLabels[k8sutils.ComponentLabel] = ComponentName
	logServerLabels[k8sutils.NameLabel] = logserverDeploymentName

	var dbSecName string
	if instance.Spec.Db.DatabaseSecret == "" {
		dbSecret := i.createDbSecret(instance.Namespace, dbLabels)
		controllerutil.SetControllerReference(instance, dbSecret, i.Client.Scheme())
		if err = i.Client.Create(ctx, dbSecret); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create secret: %w", err)
		}
		dbSecName = dbSecret.Name
	} else {
		dbSecName = instance.Spec.Db.DatabaseSecret
	}

	var trillPVC string
	if instance.Spec.Db.Create {
		if instance.Spec.Db.PvcName == "" {
			// PVC does not exist, create a new one
			i.Logger.V(1).Info("Creating new PVC")
			pvc := k8sutils.CreatePVC(instance.Namespace, "trillian-mysql", "5Gi")
			controllerutil.SetControllerReference(instance, pvc, i.Client.Scheme())
			if err = i.Client.Create(ctx, pvc); err != nil {
				instance.Status.Phase = rhtasv1alpha1.PhaseError
				return instance, fmt.Errorf("could not create MySQL PVC: %w", err)
			}
			trillPVC = pvc.Name
		}
		// TODO: add status field
	} else {
		trillPVC = instance.Spec.Db.PvcName
	}

	if instance.Spec.Db.Create {
		sa := kubernetes.CreateServiceAccount(instance.Namespace, dbServiceAccountName, dbLabels)
		controllerutil.SetControllerReference(instance, sa, i.Client.Scheme())
		if err = i.Client.Create(ctx, sa); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create trillian DB sa: %w", err)
		}

		db := trillianUtils.CreateTrillDb(instance.Namespace, constants.TrillianDbImage, dbDeploymentName, trillPVC, dbSecName, dbLabels, sa.Name)
		controllerutil.SetControllerReference(instance, db, i.Client.Scheme())
		if err = i.Client.Create(ctx, db); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create trillian DB: %w", err)
		}

		mysql := k8sutils.CreateService(instance.Namespace, "trillian-mysql", 3306, dbLabels)
		controllerutil.SetControllerReference(instance, mysql, i.Client.Scheme())
		if err = i.Client.Create(ctx, mysql); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create service: %w", err)
		}
	}

	// Log Server
	svcName := "trillian-logserver"
	serverPort := 8091

	logserverService := k8sutils.CreateService(instance.Namespace, svcName, serverPort, logServerLabels)
	controllerutil.SetControllerReference(instance, logserverService, i.Client.Scheme())
	if err = i.Client.Create(ctx, logserverService); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}
	instance.Status.Url = fmt.Sprintf("%s.%s.svc:%d", logserverService.Name, logserverService.Namespace, serverPort)

	serverSA := kubernetes.CreateServiceAccount(instance.Namespace, logserverServiceAccountName, logServerLabels)
	controllerutil.SetControllerReference(instance, serverSA, i.Client.Scheme())
	if err = i.Client.Create(ctx, serverSA); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create trillian logserver sa: %w", err)
	}

	server := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianServerImage, logserverDeploymentName, dbSecName, logServerLabels, serverSA.Name)
	controllerutil.SetControllerReference(instance, server, i.Client.Scheme())
	server.Spec.Template.Spec.Containers[0].Ports = append(server.Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{
		Protocol:      corev1.ProtocolTCP,
		ContainerPort: 8090,
	})
	if err = i.Client.Create(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	// Log Signer
	signerSA := kubernetes.CreateServiceAccount(instance.Namespace, logsignerServiceAccountName, logSignerLabels)
	controllerutil.SetControllerReference(instance, signerSA, i.Client.Scheme())
	if err = i.Client.Create(ctx, signerSA); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create trillian logsigner sa: %w", err)
	}

	signer := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianLogSignerImage, logsignerDeploymentName, dbSecName, logSignerLabels, signerSA.Name)
	controllerutil.SetControllerReference(instance, signer, i.Client.Scheme())
	signer.Spec.Template.Spec.Containers[0].Args = append(signer.Spec.Template.Spec.Containers[0].Args, "--force_master=true")
	if err = i.Client.Create(ctx, signer); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return instance, nil

}

func (i createAction) createDbSecret(namespace string, labels map[string]string) *corev1.Secret {
	// Define a new Secret object
	var rootPass []byte
	var mysqlPass []byte
	rootPass = common.GeneratePassword(12)
	mysqlPass = common.GeneratePassword(12)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rhtas",
			Namespace:    namespace,
			Labels:       labels,
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"mysql-root-password": rootPass,
			"mysql-password":      mysqlPass,
			"mysql-database":      []byte("trillian"),
			"mysql-user":          []byte("mysql"),
			"mysql-port":          []byte("3306"),
			"mysql-host":          []byte("trillian-mysql"),
		},
	}
}
