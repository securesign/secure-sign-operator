package api

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewCreateServiceAction() action.Action[*rhtasv1.Console] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "api create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	l := labels.For(actions.ApiComponentName, actions.ApiDeploymentName, instance.Name)

	tlsAnnotations := map[string]string{}
	if specTLS(instance).CertRef == nil {
		tlsAnnotations[annotations.TLS] = fmt.Sprintf(actions.ApiTLSSecret, instance.Name)
	}

	ports := []v1.ServicePort{
		{
			Name:       actions.ApiPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       actions.ApiPort,
			TargetPort: intstr.FromString(actions.ApiPortName),
		},
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: actions.ApiDeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureServiceSpec(l, ports...),
		ensure.ControllerReference[*v1.Service](instance, i.Client),
		ensure.Labels[*v1.Service](slices.Collect(maps.Keys(l)), l),
		//TLS: Annotate service
		ensure.Optional(kubernetes.IsOpenShift(), ensure.Annotations[*v1.Service]([]string{annotations.TLS}, tlsAnnotations)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ApiCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
