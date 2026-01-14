package trillianUtils

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func EnsureServerDeployment(instance *v1alpha1.Trillian, labels map[string]string, caPath string) []func(*apps.Deployment) error {
	return append([]func(deployment *apps.Deployment) error{
		ensureDeployment(instance,
			images.Registry.Get(images.TrillianServer),
			actions.LogserverDeploymentName,
			actions.RBACServerName,
			labels),
		ensureProbes(actions.LogserverDeploymentName),
		deployment.PodRequirements(instance.Spec.LogServer.PodRequirements, actions.LogserverDeploymentName),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.LogserverDeploymentName)},
		EnsureDB(instance, actions.LogserverDeploymentName, caPath)...)
}

func EnsureSignerDeployment(instance *v1alpha1.Trillian, labels map[string]string, caPath string) []func(*apps.Deployment) error {
	return append([]func(deployment *apps.Deployment) error{
		ensureDeployment(instance,
			images.Registry.Get(images.TrillianLogSigner),
			actions.LogsignerDeploymentName,
			actions.RBACSignerName,
			labels,
			"--election_system=k8s", "--lock_namespace=$(NAMESPACE)", "--lock_holder_identity=$(POD_NAME)", "--master_hold_interval=5s", "--master_hold_jitter=15s"),
		ensureProbes(actions.LogsignerDeploymentName),
		deployment.PodRequirements(instance.Spec.LogSigner.PodRequirements, actions.LogsignerDeploymentName),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), actions.LogsignerDeploymentName),
	},
		EnsureDB(instance, actions.LogsignerDeploymentName, caPath)...)
}

func ensureProbes(containerName string) func(*apps.Deployment) error {
	return func(deployment *apps.Deployment) error {
		container := kubernetes.FindContainerByNameOrCreate(&deployment.Spec.Template.Spec, containerName)

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/healthz"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(actions.MetricsPort)

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.ReadinessProbe.HTTPGet.Path = "/healthz"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(actions.MetricsPort)
		container.ReadinessProbe.InitialDelaySeconds = 10
		return nil
	}
}

func ensureDeployment(instance *v1alpha1.Trillian, image string, name string, sa string, labels map[string]string, args ...string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		if instance.Status.Db.DatabaseSecretRef == nil && utils.OptionalBool(instance.Spec.Db.Create) {
			return errors.New("reference to database secret is not set")
		}

		spec := &dp.Spec
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, name)
		container.Image = image

		container.Args = append([]string{
			"--storage_system=" + instance.Spec.Db.Provider,
			"--quota_system=" + instance.Spec.Db.Provider,
			"--rpc_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.ServerPort)),
			"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.MetricsPort)),
			"--alsologtostderr",
		}, args...)

		if instance.Spec.MaxRecvMessageSize != nil {
			container.Args = append(container.Args, "--max_msg_size_bytes", fmt.Sprintf("%d", *instance.Spec.MaxRecvMessageSize))
		}

		podNameEnv := kubernetes.FindEnvByNameOrCreate(container, "POD_NAME")
		podNameEnv.ValueFrom = &core.EnvVarSource{
			FieldRef: &core.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.name",
			},
		}

		namespaceEnv := kubernetes.FindEnvByNameOrCreate(container, "NAMESPACE")
		namespaceEnv.ValueFrom = &core.EnvVarSource{
			FieldRef: &core.ObjectFieldSelector{
				APIVersion: "v1",
				FieldPath:  "metadata.namespace",
			},
		}

		port := kubernetes.FindPortByNameOrCreate(container, "8091-tcp")
		port.ContainerPort = actions.ServerPort
		port.Protocol = core.ProtocolTCP

		if instance.Spec.Monitoring.Enabled {
			monitoring := kubernetes.FindPortByNameOrCreate(container, "monitoring")
			monitoring.ContainerPort = actions.MetricsPort
			monitoring.Protocol = core.ProtocolTCP
		}
		return nil
	}
}

func EnsureTLS(tlsConfig v1alpha1.TLS, name string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		if err := deployment.TLS(tlsConfig, name)(dp); err != nil {
			return err
		}

		container := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, name)

		container.Args = append(container.Args, "--tls_cert_file", tls.TLSCertPath)

		if container.ReadinessProbe != nil {
			container.ReadinessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		if container.LivenessProbe != nil {
			container.LivenessProbe.HTTPGet.Scheme = core.URISchemeHTTPS
		}

		container.Args = append(container.Args, "--tls_key_file", tls.TLSKeyPath)

		return nil
	}
}
