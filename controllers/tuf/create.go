package tuf

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/fulcio/utils"
	tufutils "github.com/securesign/operator/controllers/tuf/utils"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	tufDeploymentName = "tuf"
	ComponentName     = "tuf"
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

func (i createAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseCreating
}

func (i createAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	var err error
	labels := kubernetes.FilterCommonLabels(instance.Labels)
	labels["app.kubernetes.io/component"] = ComponentName
	labels["app.kubernetes.io/name"] = tufDeploymentName

	fulcio, err := utils.FindFulcio(ctx, i.Client, instance.Namespace, kubernetes.FilterCommonLabels(instance.Labels))
	if err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not find Fulcio: %s", err)
	}

	db := tufutils.CreateTufDeployment(instance.Namespace, tufDeploymentName, fulcio.Spec.Certificate.SecretName, "rekor-public-key", labels)
	controllerutil.SetControllerReference(instance, db, i.Client.Scheme())
	if err = i.Client.Create(ctx, db); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create TUF: %w", err)
	}

	svc := kubernetes.CreateService(instance.Namespace, "tuf", 8080, labels)
	//patch the pregenerated service
	svc.Spec.Ports[0].Port = 80
	controllerutil.SetControllerReference(instance, svc, i.Client.Scheme())
	if err = i.Client.Create(ctx, svc); err != nil {
		instance.Status.Phase = rhtasv1alpha1.PhaseError
		return instance, fmt.Errorf("could not create service: %w", err)
	}

	if instance.Spec.External {
		// TODO: do we need to support ingress?
		route := kubernetes.CreateRoute(*svc, "tuf", labels)
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
