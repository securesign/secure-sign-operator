package clidownload

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/images"
	cLabels "github.com/securesign/operator/internal/labels"
	cutils "github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"

	"github.com/go-logr/logr"
	rhtasv1 "github.com/securesign/operator/api/v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cliServerNs        = "trusted-artifact-signer"
	cliServerName      = "cli-server"
	cliServerComponent = "client-server"
	cliServerPortName  = "http"
	cliServerPort      = 8080
)

var (
	CliHostName string
)

//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=create
//+kubebuilder:rbac:groups=core,resources=namespaces,resourceNames=trusted-artifact-signer,verbs=update;patch;delete

type Component struct {
	Client client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (c *Component) Start(ctx context.Context) error {
	labels := map[string]string{
		cLabels.LabelAppPartOf:    constants.AppName,
		cLabels.LabelAppComponent: cliServerComponent,
	}

	c.Log.Info("installing client server resources")

	svc := &core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerName,
			Namespace: cliServerNs,
		},
	}

	ingress := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: cliServerName, Namespace: cliServerNs},
	}

	if e := CreateResource[*core.Namespace](ctx, c.Client, c.Log,
		&core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: cliServerNs,
			}}, ensure.Labels[*core.Namespace](slices.Collect(maps.Keys(labels)), labels)); e != nil {
		return e
	}
	if e := CreateResource[*apps.Deployment](ctx, c.Client, c.Log,
		&apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cliServerName,
				Namespace: cliServerNs,
			}},
		c.ensureDeployment(labels),
		ensure.Labels[*apps.Deployment](slices.Collect(maps.Keys(labels)), labels)); e != nil {
		return e
	}

	if e := CreateResource[*core.Service](ctx, c.Client, c.Log,
		svc,
		kubernetes.EnsureServiceSpec(labels,
			core.ServicePort{
				Name:       cliServerPortName,
				Protocol:   core.ProtocolTCP,
				Port:       cliServerPort,
				TargetPort: intstr.FromInt32(cliServerPort),
			}),
		ensure.Labels[*core.Service](slices.Collect(maps.Keys(labels)), labels),
	); e != nil {
		return e
	}

	if e := CreateResource[*v1.Ingress](ctx, c.Client, c.Log,
		ingress,
		kubernetes.EnsureIngressSpec(ctx, c.Client, *svc, rhtasv1.ExternalAccess{Host: CliHostName}, cliServerPortName),
		ensure.Optional(kubernetes.IsOpenShift(), kubernetes.EnsureIngressTLS()),
		ensure.Labels[*v1.Ingress](slices.Collect(maps.Keys(labels)), labels),
	); e != nil {
		return e
	}

	return nil
}

func CreateResource[T client.Object](ctx context.Context, cli client.Client, log logr.Logger, obj T, fn ...func(T) error) error {
	if result, err := kubernetes.CreateOrUpdate(ctx, cli,
		obj,
		fn...,
	); err != nil {
		return fmt.Errorf("could not create %s: %w", obj.GetObjectKind(), err)
	} else {
		if result != controllerutil.OperationResultNone {
			log.Info("resource", "name", obj.GetObjectKind(), "namespace", obj.GetNamespace(), "operation", string(result))
		}
	}
	return nil
}

func (c *Component) ensureDeployment(labels map[string]string) func(*apps.Deployment) error {
	return func(dp *apps.Deployment) error {

		spec := &dp.Spec
		spec.Replicas = cutils.Pointer[int32](1)
		spec.Selector = &metav1.LabelSelector{
			MatchLabels: labels,
		}

		template := &spec.Template
		template.Labels = labels

		container := kubernetes.FindContainerByNameOrCreate(&template.Spec, cliServerName)
		container.Image = images.Registry.Get(images.ClientServer)
		container.ImagePullPolicy = core.PullAlways

		port := kubernetes.FindPortByNameOrCreate(container, "http")
		port.ContainerPort = 8080
		port.Protocol = core.ProtocolTCP

		return nil
	}
}
