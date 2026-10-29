package trillianUtils

import (
	"errors"
	"fmt"
	"strconv"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

func EnsureDeployment(instance *rhtasv1.Trillian, image, name string) traits.Trait[*appsv1ac.DeploymentApplyConfiguration] {
	return func(dp *appsv1ac.DeploymentApplyConfiguration) error {
		if instance.Status.Db.DatabaseSecretRef == nil && utils.OptionalBool(instance.Spec.Db.Create) {
			return errors.New("reference to database secret is not set")
		}

		healthProbe := func() *corev1ac.ProbeApplyConfiguration {
			return corev1ac.Probe().WithHTTPGet(corev1ac.HTTPGetAction().
				WithPath(constants.HealthzPath).WithPort(intstr.FromInt32(actions.MetricsPort)))
		}
		container := corev1ac.Container().WithName(name).WithImage(image).
			WithArgs(
				"--storage_system", instance.Spec.Db.Provider,
				"--quota_system", instance.Spec.Db.Provider,
				"--rpc_endpoint", "0.0.0.0:"+strconv.Itoa(int(actions.ServerPort)),
				"--http_endpoint", "0.0.0.0:"+strconv.Itoa(int(actions.MetricsPort)),
				"--alsologtostderr",
			).
			WithPorts(corev1ac.ContainerPort().WithName("8091-tcp").WithContainerPort(actions.ServerPort).WithProtocol(core.ProtocolTCP)).
			WithEnv(corev1ac.EnvVar().WithName("POD_NAME").WithValueFrom(corev1ac.EnvVarSource().
				WithFieldRef(corev1ac.ObjectFieldSelector().WithAPIVersion("v1").WithFieldPath("metadata.name")))).
			WithEnv(corev1ac.EnvVar().WithName("NAMESPACE").WithValueFrom(corev1ac.EnvVarSource().
				WithFieldRef(corev1ac.ObjectFieldSelector().WithAPIVersion("v1").WithFieldPath("metadata.namespace")))).
			WithLivenessProbe(healthProbe().WithPeriodSeconds(10).WithTimeoutSeconds(1).WithFailureThreshold(3)).
			WithReadinessProbe(healthProbe().WithPeriodSeconds(10).WithTimeoutSeconds(1).WithFailureThreshold(3)).
			WithStartupProbe(healthProbe().WithPeriodSeconds(5).WithTimeoutSeconds(5).WithFailureThreshold(12))

		if instance.Spec.MaxRecvMessageSize != nil {
			container.WithArgs("--max_msg_size_bytes", fmt.Sprintf("%d", *instance.Spec.MaxRecvMessageSize))
		}

		if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
			container.WithPorts(corev1ac.ContainerPort().WithName("monitoring").WithContainerPort(actions.MetricsPort).WithProtocol(core.ProtocolTCP))
		}
		dp.WithSpec(appsv1ac.DeploymentSpec().
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(dp.Labels)).
			WithStrategy(appsv1ac.DeploymentStrategy().WithType(apps.RollingUpdateDeploymentStrategyType)).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(dp.Labels).
				WithSpec(corev1ac.PodSpec().WithContainers(container))))
		return nil
	}
}

func EnsureTLS(tlsConfig rhtasv1.TLS, name string) traits.Trait[*corev1ac.PodSpecApplyConfiguration] {
	return func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
		if err := traits.TLS(tlsConfig, name)(podSpec); err != nil {
			return err
		}
		container := kubernetes.FindContainerByName(podSpec, name)
		container.WithArgs("--tls_cert_file", tls.TLSCertPath, "--tls_key_file", tls.TLSKeyPath)
		for _, probe := range []*corev1ac.ProbeApplyConfiguration{container.ReadinessProbe, container.LivenessProbe, container.StartupProbe} {
			if probe != nil && probe.HTTPGet != nil {
				probe.HTTPGet.WithScheme(core.URISchemeHTTPS)
			}
		}
		return nil
	}
}
