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
	consoleUtils "github.com/securesign/operator/internal/controller/console/utils"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/images"
	core "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	tufURL := i.resolveTufUrl(ctx, instance)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.ApiDeploymentName,
				Namespace: instance.Namespace,
			},
		},
		append([]func(*apps.Deployment) error{
			i.ensureAPIDeployment(instance, actions.RBACApiName, labels, tufURL),
			ensure.ControllerReference[*v1.Deployment](instance, i.Client),
			ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
			deployment.Proxy(),
			deployment.PodRequirements(instance.Spec.Api.PodRequirements, actions.ApiComponentName),
			deployment.TrustedCA(instance.GetTrustedCA(), actions.ApiComponentName),
			ensure.Optional(consoleUtils.UseTLSApi(instance), i.ensureTLS(statusTLS(instance))),
			deployment.PodSecurityContext(),
		}, ensureDbAuth(instance, actions.ApiDeploymentName)...)...,
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
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) ensureAPIDeployment(instance *rhtasv1alpha1.Console, sa string, labels map[string]string, tufURL string,
) func(*apps.Deployment) error {

	return func(dp *apps.Deployment) error {
		tufServerHost := tufURL

		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		replicas := int32(1)
		spec.Replicas = &replicas

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		dbCheckCmd := fmt.Sprintf("mysqladmin ping -h%s --silent", actions.DbDeploymentName)
		if consoleUtils.UseTLSDb(instance) {
			dbCheckCmd += " --ssl"
		}

		initContainer := kubernetes.FindInitContainerByNameOrCreate(&template.Spec, "wait-for-console-db-tuf")
		initContainer.Image = images.Registry.Get(images.ConsoleDb)

		initContainer.Command = []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(`
                echo "Waiting for console database...";
                until %s > /dev/null 2>&1; do
                    echo "Waiting for the console database to be ready...";
                    sleep 5;
                done;
                echo "Waiting for TUF server...";
                until curl %s > /dev/null 2>&1; do
                    echo "TUF server not ready...";
                    sleep 5;
                done;
                echo "tuf-init completed."
            `, dbCheckCmd, tufServerHost),
		}

		// Apply Auth to init container
		ref := &template.Spec
		if instance.Spec.Auth != nil {
			err := ensure.ContainerAuth(initContainer, instance.Spec.Auth)(ref)
			if err != nil {
				return err
			}
		}
		// Apply DatabaseSecretRef auth to init container
		if instance.Status.Db.DatabaseSecretRef != nil {
			err := ensure.ContainerAuth(initContainer, dbSecretToAuth(instance.Status.Db.DatabaseSecretRef))(ref)
			if err != nil {
				return err
			}
		}

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, actions.ApiDeploymentName)
		container.Image = images.Registry.Get(images.ConsoleApi)

		tufRepoUrlEnv := kubernetes.FindEnvByNameOrCreate(container, "TUF_REPO_URL")
		tufRepoUrlEnv.Value = tufServerHost

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

func (i deployAction) resolveTufUrl(ctx context.Context, instance *rhtasv1alpha1.Console) string {
	if instance.Spec.Api.Tuf.Address != "" {
		url := instance.Spec.Api.Tuf.Address
		if instance.Spec.Api.Tuf.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Api.Tuf.Port)
		}
		return url
	}

	// Try to get the TUF instance and use its external URL if available
	// Typically, the TUF instance has the same name as the parent Securesign instance
	tufName := instance.Name
	if ownerRefs := instance.GetOwnerReferences(); len(ownerRefs) > 0 {
		for _, owner := range ownerRefs {
			if owner.Kind == "Securesign" {
				tufName = owner.Name
				break
			}
		}
	}

	tuf := &rhtasv1alpha1.Tuf{}
	if err := i.Client.Get(ctx, client.ObjectKey{Name: tufName, Namespace: instance.Namespace}, tuf); err == nil {
		if tuf.Status.Url != "" {
			return tuf.Status.Url
		}
	}

	return fmt.Sprintf("http://tuf.%s.svc", instance.Namespace)
}

func (i deployAction) ensureTLS(tlsConfig rhtasv1alpha1.TLS) func(deployment *apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		if err := deployment.TLS(tlsConfig, actions.ApiComponentName)(dp); err != nil {
			return err
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, actions.ApiDeploymentName)

		container.Args = []string{
			"--tls-cert=/var/run/secrets/tas/tls.crt",
			"--tls-key=/var/run/secrets/tas/tls.key",
		}

		if container.ReadinessProbe != nil && container.ReadinessProbe.HTTPGet != nil {
			container.ReadinessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		if container.LivenessProbe != nil && container.LivenessProbe.HTTPGet != nil {
			container.LivenessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		return nil
	}
}
