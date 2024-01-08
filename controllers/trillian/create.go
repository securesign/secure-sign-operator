package trillian

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	trillianUtils "github.com/securesign/operator/controllers/trillian/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	dbDeploymentName        = "trillian-db"
	logserverDeploymentName = "trillian-logserver"
	logsignerDeploymentName = "trillian-logsigner"

	ComponentName = "trillian"
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

func (i createAction) CanHandle(trillian *rhtasv1alpha1.Trillian) bool {
	return trillian.Status.Phase == rhtasv1alpha1.PhaseNone
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) (*rhtasv1alpha1.Trillian, error) {
	//log := ctrllog.FromContext(ctx)
	var err error

	dbLabels := kubernetes.FilterCommonLabels(instance.Labels)
	dbLabels["app.kubernetes.io/component"] = ComponentName
	dbLabels["app.kubernetes.io/name"] = dbDeploymentName

	logSignerLabels := kubernetes.FilterCommonLabels(instance.Labels)
	logSignerLabels["app.kubernetes.io/component"] = ComponentName
	logSignerLabels["app.kubernetes.io/name"] = logsignerDeploymentName

	logServerLabels := kubernetes.FilterCommonLabels(instance.Labels)
	logServerLabels["app.kubernetes.io/component"] = ComponentName
	logServerLabels["app.kubernetes.io/name"] = logserverDeploymentName

	dbSecret := i.createDbSecret(instance.Namespace, dbLabels)
	controllerutil.SetControllerReference(instance, dbSecret, i.Client.Scheme())
	if err = i.Client.Create(ctx, dbSecret); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create secret: %w", err)
	}

	var trillPVC string
	if instance.Spec.PvcName == "" {
		pvc := kubernetes.CreatePVC(instance.Namespace, "trillian-mysql", "5Gi")
		controllerutil.SetControllerReference(instance, pvc, i.Client.Scheme())
		if err = i.Client.Create(ctx, pvc); err != nil {
			instance.Status.Phase = rhtasv1alpha1.PhaseError
			return instance, fmt.Errorf("could not create pvc: %w", err)
		}
		trillPVC = pvc.Name
	} else {
		trillPVC = instance.Spec.PvcName
	}

	db := trillianUtils.CreateTrillDb(instance.Namespace, constants.TrillianDbImage, dbDeploymentName, trillPVC, dbSecret.Name, dbLabels)
	controllerutil.SetControllerReference(instance, db, i.Client.Scheme())
	if err = i.Client.Create(ctx, db); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create trillian DB: %w", err)
	}

	mysql := kubernetes.CreateService(instance.Namespace, "trillian-mysql", 3306, dbLabels)
	controllerutil.SetControllerReference(instance, mysql, i.Client.Scheme())
	if err = i.Client.Create(ctx, mysql); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}

	// Log Server
	server := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianServerImage, logserverDeploymentName, dbSecret.Name, logServerLabels)
	controllerutil.SetControllerReference(instance, server, i.Client.Scheme())
	server.Spec.Template.Spec.Containers[0].Ports = append(server.Spec.Template.Spec.Containers[0].Ports, corev1.ContainerPort{
		Protocol:      corev1.ProtocolTCP,
		ContainerPort: 8090,
	})
	if err = i.Client.Create(ctx, server); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create job: %w", err)
	}

	serverPort := 8091
	logserverService := kubernetes.CreateService(instance.Namespace, "trillian-logserver", serverPort, logServerLabels)
	controllerutil.SetControllerReference(instance, logserverService, i.Client.Scheme())
	if err = i.Client.Create(ctx, logserverService); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}
	instance.Status.Url = fmt.Sprintf("%s.%s.svc:%d", logserverService.Name, logserverService.Namespace, serverPort)

	// Log Signer
	signer := trillianUtils.CreateTrillDeployment(instance.Namespace, constants.TrillianLogSignerImage, logsignerDeploymentName, dbSecret.Name, logSignerLabels)
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
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rhtas",
			Namespace:    namespace,
			Labels:       labels,
		},
		Type: "Opaque",
		Data: map[string][]byte{
			// generate a random password for the mysql root user and the mysql password
			// TODO - use a random password generator
			"mysql-root-password": []byte("password"),
			"mysql-password":      []byte("password"),
			"mysql-database":      []byte("trillian"),
			"mysql-user":          []byte("mysql"),
		},
	}
}
