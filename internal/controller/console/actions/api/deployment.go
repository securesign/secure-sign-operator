package api

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewDeployAction() action.Action[*rhtasv1.Console] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "api deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	tufURL := resolveTufUrl(instance)

	l := labels.For(actions.ApiComponentName, actions.ApiDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ApiDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		ensure.ControllerReference[*apps.Deployment](instance, i.Client),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(l)), l),
		ensureApiDeployment(instance, l, tufURL),
		ensureTUFWaitInitContainer(tufURL),
		ensureApiProbes(),
		deployment.PodRequirements(instance.Spec.Api.PodRequirements, actions.ApiDeploymentName),
		deployment.Proxy(),
		deployment.GODEBUG(instance.GetAnnotations()),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.ApiDeploymentName),
		deployment.PodSecurityContext(),
		ensure.Optional(
			statusTLS(instance).CertRef != nil,
			ensureTLS(statusTLS(instance)),
		),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console api: %w", err), instance, metav1.Condition{
			Type:    actions.ApiCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Failure.String(),
			Message: err.Error(),
		})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ApiCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Deployment created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}

func resolveTufUrl(instance *rhtasv1.Console) string {
	if instance.Spec.Api.Tuf.Address != "" {
		url := instance.Spec.Api.Tuf.Address
		if instance.Spec.Api.Tuf.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Api.Tuf.Port)
		}
		return url
	}
	return fmt.Sprintf("http://tuf.%s.svc", instance.Namespace)
}

func ensureApiDeployment(instance *rhtasv1.Console, labels map[string]string, tufURL string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}
		spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		replicas := int32(1)
		spec.Replicas = &replicas

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = actions.RBACApiName

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.ApiDeploymentName)
		container.Image = images.Registry.Get(images.ConsoleApi)
		container.Args = []string{"--tuf-repo-url=" + tufURL}

		port := kubernetes.FindPortByNameOrCreate(container, actions.ApiPortName)
		port.ContainerPort = actions.ApiPort
		port.Protocol = core.ProtocolTCP

		return nil
	}
}

func ensureTUFWaitInitContainer(tufURL string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		initContainer := kubernetes.FindInitContainerByNameOrCreate(&dp.Spec.Template.Spec, "wait-for-tuf")
		initContainer.Image = images.Registry.Get(images.TrillianNetcat)
		initContainer.Command = []string{"sh", "-c"}
		initContainer.Args = []string{
			fmt.Sprintf(`
echo "Waiting for TUF server...";
until curl -f %s/root.json > /dev/null 2>&1; do
    echo "TUF server not ready...";
    sleep 5;
done;
echo "TUF server is ready."
`, tufURL),
		}

		return nil
	}
}

func ensureApiProbes() func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ApiDeploymentName)

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/healthz"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(actions.ApiPort)
		container.LivenessProbe.InitialDelaySeconds = 20
		container.LivenessProbe.PeriodSeconds = 10
		container.LivenessProbe.TimeoutSeconds = 5
		container.LivenessProbe.FailureThreshold = 3

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/healthz"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(actions.ApiPort)
		container.ReadinessProbe.InitialDelaySeconds = 10
		container.ReadinessProbe.PeriodSeconds = 10
		container.ReadinessProbe.TimeoutSeconds = 5
		container.ReadinessProbe.FailureThreshold = 3

		return nil
	}
}

func ensureTLS(tlsConfig rhtasv1.TLS) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		if err := deployment.TLS(tlsConfig, actions.ApiDeploymentName)(dp); err != nil {
			return err
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ApiDeploymentName)

		container.Args = append(container.Args,
			"--tls-cert="+tls.TLSCertPath,
			"--tls-key="+tls.TLSKeyPath,
		)

		if container.ReadinessProbe != nil && container.ReadinessProbe.HTTPGet != nil {
			container.ReadinessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		if container.LivenessProbe != nil && container.LivenessProbe.HTTPGet != nil {
			container.LivenessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		return nil
	}
}
