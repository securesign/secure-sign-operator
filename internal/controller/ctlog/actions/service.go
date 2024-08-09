package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServiceAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err     error
		updated bool
	)

	labels := constants.LabelsFor(ComponentName, ComponentName, instance.Name)

	svc := kubernetes.CreateService(instance.Namespace, ComponentName, ServerPortName, ServerPort, ServerTargetPort, labels)
	signingKeySecret, _ := k8sutils.GetSecret(i.Client, "openshift-service-ca", "signing-key")
	if instance.Spec.Monitoring.Enabled {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:       MetricsPortName,
			Protocol:   corev1.ProtocolTCP,
			Port:       MetricsPort,
			TargetPort: intstr.FromInt32(MetricsPort),
		})
	}
	if err = controllerutil.SetControllerReference(instance, svc, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Service: %w", err))
	}
	if updated, err = i.Ensure(ctx, svc); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	//TLS: Annotate service
	if signingKeySecret != nil && instance.Spec.TLSCertificate.CertRef == nil {
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = instance.Name + "-ctlog-tls-secret"
		err := i.Client.Update(ctx, svc)
		if err != nil {
			return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not annotate service: %w", err), instance)
		}
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Service created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
