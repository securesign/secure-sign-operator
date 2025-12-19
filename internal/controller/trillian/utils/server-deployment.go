package trillianUtils

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const initContainerName = "wait-for-trillian-db"

func EnsureServerDeployment(instance *v1alpha1.Trillian, labels map[string]string) []func(*apps.Deployment) error {
	return []func(deployment *apps.Deployment) error{
		ensureDeployment(instance,
			images.Registry.Get(images.TrillianServer),
			actions.LogserverDeploymentName,
			actions.RBACServerName,
			labels),
		ensureInitContainer(instance),
		ensureProbes(actions.LogserverDeploymentName),
		deployment.PodRequirements(instance.Spec.LogServer.PodRequirements, actions.LogserverDeploymentName),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), initContainerName, actions.LogserverDeploymentName),
		deployment.Auth(actions.LogserverDeploymentName, instance.Spec.Auth),
	}
}

func EnsureSignerDeployment(instance *v1alpha1.Trillian, labels map[string]string) []func(*apps.Deployment) error {
	return []func(deployment *apps.Deployment) error{
		ensureDeployment(instance,
			images.Registry.Get(images.TrillianLogSigner),
			actions.LogsignerDeploymentName,
			actions.RBACSignerName,
			labels,
			"--election_system=k8s", "--lock_namespace=$(NAMESPACE)", "--lock_holder_identity=$(POD_NAME)", "--master_hold_interval=5s", "--master_hold_jitter=15s"),
		ensureInitContainer(instance),
		ensureProbes(actions.LogsignerDeploymentName),
		deployment.PodRequirements(instance.Spec.LogSigner.PodRequirements, actions.LogsignerDeploymentName),
		deployment.Proxy(),
		deployment.TrustedCA(instance.GetTrustedCA(), initContainerName, actions.LogsignerDeploymentName),
		deployment.Auth(actions.LogsignerDeploymentName, instance.Spec.Auth),
	}
}

func ensureInitContainer(instance *v1alpha1.Trillian) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		initContainer := kubernetes.FindInitContainerByNameOrCreate(&dp.Spec.Template.Spec, initContainerName)
		initContainer.Image = images.Registry.Get(images.TrillianNetcat)

		if err := EnsureDB(instance, ensureMysqlParams(true))(initContainer); err != nil {
			return err
		}

		return nil
	}
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
		if instance.Status.Db.DatabaseSecretRef == nil && *instance.Spec.Db.Create {
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
			"--rpc_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.ServerPort)),
			"--http_endpoint=0.0.0.0:" + strconv.Itoa(int(actions.MetricsPort)),
			"--alsologtostderr",
		}, args...)

		if instance.Spec.MaxRecvMessageSize != nil {
			container.Args = append(container.Args, "--max_msg_size_bytes", fmt.Sprintf("%d", *instance.Spec.MaxRecvMessageSize))
		}

		//Ports = containerPorts
		if err := EnsureDB(instance, ensureMysqlParams(false))(container); err != nil {
			return err
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

func ensureMysqlParams(isInitContainer bool) func(*v1alpha1.Trillian, *MySQLOptions, *core.Container) error {
	return func(instance *v1alpha1.Trillian, options *MySQLOptions, container *core.Container) error {

		hostEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_HOSTNAME")
		hostEnv.Value = options.Host

		portEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_PORT")
		portEnv.Value = options.Port

		if isInitContainer {
			container.Command = []string{
				"sh",
				"-c",
				"until nc -z -v -w30 $MYSQL_HOSTNAME $MYSQL_PORT; do echo \"Waiting for MySQL to start\"; sleep 5; done;",
			}
		} else {
			userEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_USER")
			userEnv.Value = options.User

			if instance.Status.Db.DatabaseSecretRef != nil {
				passwordEnvName, ok := ExtractEnvVarName(options.Password)
				if !ok {
					return fmt.Errorf("invalid MYSQL password env reference: %q", options.Password)
				}
				passwordEnv := kubernetes.FindEnvByNameOrCreate(container, passwordEnvName)
				passwordEnv.ValueFrom = &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						Key: instance.Status.Db.DatabaseSecretRef.PasswordKey,
						LocalObjectReference: core.LocalObjectReference{
							Name: instance.Status.Db.DatabaseSecretRef.Name,
						},
					},
				}
			}

			dbEnv := kubernetes.FindEnvByNameOrCreate(container, "MYSQL_DATABASE")
			dbEnv.Value = options.Database

			container.Args = append([]string{
				"--storage_system=mysql",
				"--quota_system=mysql",
				fmt.Sprintf(
					"--mysql_uri=$(MYSQL_USER):%s@tcp($(MYSQL_HOSTNAME):$(MYSQL_PORT))/$(MYSQL_DATABASE)", options.Password,
				),
				"--mysql_max_conns=30",
				"--mysql_max_idle_conns=10",
			}, container.Args...)
		}
		return nil
	}
}

func WithTlsDB(instance *v1alpha1.Trillian, caPath string, name string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {
		c := kubernetes.FindContainerByNameOrCreate(&dp.Spec.Template.Spec, name)
		c.Args = append(c.Args, "--mysql_tls_ca", caPath)

		mysqlServerName := "$(MYSQL_HOSTNAME)." + instance.Namespace + ".svc"
		if !*instance.Spec.Db.Create {
			mysqlServerName = "$(MYSQL_HOSTNAME)"
		}
		c.Args = append(c.Args, "--mysql_server_name", mysqlServerName)
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
