package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure/deployment"
	"github.com/securesign/operator/internal/utils/tls"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/images"
)

const containerName = "fulcio-server"

func NewDeployAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &deployAction{}
}

type deployAction struct {
	action.BaseAction
}

func (i deployAction) Name() string {
	return "deploy"
}

func (i deployAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i deployAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      DeploymentName,
				Namespace: instance.Namespace,
			},
		},
		i.ensureDeployment(instance, RBACName, labels),
		ensure.ControllerReference[*v1.Deployment](instance, i.Client),
		ensure.Labels[*v1.Deployment](slices.Collect(maps.Keys(labels)), labels),
		// need to add Fulcio's unix domain socket used for the legacy gRPC server other way it will be
		// rest v1 api will be routed through proxy
		deployment.Proxy("@fulcio-legacy-grpc-socket"),
		deployment.TrustedCA(instance.GetTrustedCA(), containerName),
		deployment.PodRequirements(instance.Spec.PodRequirements, containerName),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Fulcio: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Deployment created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

func (i deployAction) resolveCTlogUrl(instance *rhtasv1alpha1.Fulcio) (string, error) {
	if instance.Spec.Ctlog.Prefix == "" {
		return "", futils.ErrCtlogPrefixNotSpecified
	}

	if instance.Spec.Ctlog.Address != "" {
		url := instance.Spec.Ctlog.Address
		if instance.Spec.Ctlog.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Ctlog.Port)
		}
		return fmt.Sprintf("%s/%s", url, instance.Spec.Ctlog.Prefix), nil
	}

	var (
		protocol string
	)
	if tls.UseTlsClient(instance) {
		protocol = "https"
	} else {
		protocol = "http"
	}
	return fmt.Sprintf("%s://ctlog.%s.svc/%s", protocol, instance.Namespace, instance.Spec.Ctlog.Prefix), nil
}

func (i deployAction) ensureDeployment(instance *rhtasv1alpha1.Fulcio, sa string, labels map[string]string) func(deployment *v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		if instance.Status.ServerConfigRef == nil {
			return errors.New("server config ref is not specified")
		}
		if instance.Status.Certificate == nil {
			return errors.New("certificate config is not specified")
		}
		if instance.Status.Certificate.PrivateKeyRef == nil {
			return errors.New("private key secret is not specified")
		}

		if instance.Status.Certificate.CARef == nil {
			return errors.New("CA secret is not specified")
		}

		ctlogUrl, err := i.resolveCTlogUrl(instance)
		if err != nil {
			return fmt.Errorf("could not resolve CTLog url: %w", err)
		}

		args := []string{
			"serve",
			"--port=5555",
			"--grpc-port=5554",
			fmt.Sprintf("--log_type=%s", utils.GetOrDefault(instance.GetAnnotations(), annotations.LogType, string(constants.Prod))),
			"--ca=fileca",
			"--fileca-key",
			"/var/run/fulcio-secrets/key.pem",
			"--fileca-cert",
			"/var/run/fulcio-secrets/cert.pem",
			fmt.Sprintf("--ct-log-url=%s", ctlogUrl),
		}

		spec := &dp.Spec
		spec.Replicas = utils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels
		template.Spec.ServiceAccountName = sa
		template.Spec.AutomountServiceAccountToken = &[]bool{true}[0]

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, containerName)
		container.Image = images.Registry.Get(images.FulcioServer)

		if instance.Status.Certificate.PrivateKeyPasswordRef != nil {
			env := kubernetes.FindEnvByNameOrCreate(container, "PASSWORD")
			env.ValueFrom = &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					Key: instance.Status.Certificate.PrivateKeyPasswordRef.Key,
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyPasswordRef.Name,
					},
				},
			}
			args = append(args, "--fileca-key-passwd", "$(PASSWORD)")
		}

		container.Args = args

		http := kubernetes.FindPortByNameOrCreate(container, "http")
		http.ContainerPort = 5555
		http.Protocol = core.ProtocolTCP

		grpc := kubernetes.FindPortByNameOrCreate(container, "grpc")
		grpc.ContainerPort = 5554
		grpc.Protocol = core.ProtocolTCP

		if instance.Spec.Monitoring.Enabled {
			monitoringPort := kubernetes.FindPortByNameOrCreate(container, "monitoring")
			monitoringPort.ContainerPort = 2112
			monitoringPort.Protocol = core.ProtocolTCP
		}

		certMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-cert")
		certMount.MountPath = "/var/run/fulcio-secrets"
		certMount.ReadOnly = true

		configMount := kubernetes.FindVolumeMountByNameOrCreate(container, "fulcio-config")
		configMount.MountPath = "/etc/fulcio-config"

		oidcInfoMount := kubernetes.FindVolumeMountByNameOrCreate(container, "oidc-info")
		oidcInfoMount.MountPath = "/var/run/fulcio"

		config := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-config")
		if config.ConfigMap == nil {
			config.ConfigMap = &core.ConfigMapVolumeSource{}
		}
		config.ConfigMap.Name = instance.Status.ServerConfigRef.Name

		cert := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "fulcio-cert")
		if cert.Projected == nil {
			cert.Projected = &core.ProjectedVolumeSource{}
		}
		cert.Projected.Sources = []core.VolumeProjection{
			{
				Secret: &core.SecretProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.PrivateKeyRef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Certificate.PrivateKeyRef.Key,
							Path: "key.pem",
						},
					},
				},
			},
			{
				Secret: &core.SecretProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: instance.Status.Certificate.CARef.Name,
					},
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Certificate.CARef.Key,
							Path: "cert.pem",
						},
					},
				},
			},
		}

		oidcInfo := kubernetes.FindVolumeByNameOrCreate(&template.Spec, "oidc-info")
		if oidcInfo.Projected == nil {
			oidcInfo.Projected = &core.ProjectedVolumeSource{}
		}
		oidcInfo.Projected.Sources = []core.VolumeProjection{
			{
				ConfigMap: &core.ConfigMapProjection{
					LocalObjectReference: core.LocalObjectReference{
						Name: "kube-root-ca.crt",
					},
					Items: []core.KeyToPath{
						{
							Key:  "ca.crt",
							Path: "ca.crt",
							Mode: ptr.To(int32(0666)),
						},
					},
				},
			},
		}

		if container.LivenessProbe == nil {
			container.LivenessProbe = &core.Probe{}
		}
		if container.LivenessProbe.HTTPGet == nil {
			container.LivenessProbe.HTTPGet = &core.HTTPGetAction{}
		}
		container.LivenessProbe.HTTPGet.Path = "/healthz"
		container.LivenessProbe.HTTPGet.Port = intstr.FromInt32(5555)

		if container.ReadinessProbe == nil {
			container.ReadinessProbe = &core.Probe{}
		}
		if container.ReadinessProbe.HTTPGet == nil {
			container.ReadinessProbe.HTTPGet = &core.HTTPGetAction{}
		}

		container.ReadinessProbe.HTTPGet.Path = "/healthz"
		container.ReadinessProbe.HTTPGet.Port = intstr.FromInt32(5555)

		return nil
	}
}
