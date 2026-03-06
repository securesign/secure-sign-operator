package api

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"

	"github.com/securesign/operator/internal/controller/console/actions"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/images"
	core "k8s.io/api/core/v1"
)

func NewDeployAction() action.Action[*rhtasv1alpha1.Console] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Console) bool {
	return instance.Spec.Enabled && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.ApiComponentName, actions.ApiDeploymentName, instance.Name)

	// caPath, err := tls.CAPath(ctx, i.Client, instance)
	// if err != nil {
	// 	return i.Error(ctx, fmt.Errorf("failed to get CA path: %w", err), instance)
	// }

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ApiDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureAPIDeployment(instance, actions.RBACApiName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		deployment.Proxy(),
		deployment.PodRequirements(instance.Spec.Api.PodRequirements, actions.ApiComponentName),
		deployment.PodSecurityContext(),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create console Api: %w", err), instance, metav1.Condition{
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
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureAPIDeployment(instance *rhtasv1alpha1.Console, sa string, labels map[string]string,
) func(*apps.Deployment) error {

	return func(dp *apps.Deployment) error {
		tufServerHost := i.resolveTufUrl(instance)

		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		replicas := int32(1)
		spec.Replicas = &replicas

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		initContainer := kubernetes.FindInitContainerByNameOrCreate(&template.Spec, "wait-for-console-db-tuf")
		initContainer.Image = images.Registry.Get(images.ConsoleDb)

		initContainer.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`
                echo "Waiting for rekor-server...";
                until mysqladmin ping -h%s --silent > /dev/null 2>&1; do
                    echo "Waiting for the console database to be ready...";
                    sleep 5;
                done;
                echo "Waiting for TUF server...";
                until curl %s > /dev/null 2>&1; do
                    echo "TUF server not ready...";
                    sleep 5;
                done;
                echo "tuf-init completed."
            `, actions.DbDeploymentName, tufServerHost),
		}

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.ApiDeploymentName)
		container.Image = images.Registry.Get(images.ConsoleApi)

		tufRepoUrlEnv := kubernetes.FindEnvByNameOrCreate(container, "TUF_REPO_URL")
		tufRepoUrlEnv.Value = tufServerHost

		dsnEnv := kubernetes.FindEnvByNameOrCreate(container, "DB_DSN")
		dsnEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretDsn,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.DatabaseSecretRef.Name,
				},
			},
		}

		userEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_USER")
		userEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretUser,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.DatabaseSecretRef.Name,
				},
			},
		}

		passwordEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PASSWORD")
		passwordEnv.ValueFrom = &core.EnvVarSource{
			SecretKeyRef: &core.SecretKeySelector{
				Key: actions.SecretPassword,
				LocalObjectReference: core.LocalObjectReference{
					Name: instance.Status.DatabaseSecretRef.Name,
				},
			},
		}

		sslCertDirEnv := kubernetes.FindEnvByNameOrCreate(container, "SSL_CERT_DIR")
		sslCertDirEnv.Value = "/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount"

		port := kubernetes.FindPortByNameOrCreate(container, "http")
		port.ContainerPort = actions.ApiServerPort
		port.Protocol = core.ProtocolTCP

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/healthz"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(actions.ApiServerPort)

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}

		container.ReadinessProbe.HTTPGet.Path = "/healthz"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(actions.ApiServerPort)

		return nil
	}
}

func (i deployAction) resolveTufUrl(instance *rhtasv1alpha1.Console) string {
	if instance.Spec.Api.Tuf.Address != "" {
		url := instance.Spec.Api.Tuf.Address
		if instance.Spec.Api.Tuf.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Api.Tuf.Port)
		}
		return url
	}
	return fmt.Sprintf("http://tuf.%s.svc", instance.Namespace)
}
