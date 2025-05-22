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
	consolev1 "github.com/openshift/api/console/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
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

type Arch struct {
	linkTemplate, descriptionTemplate string
}

var (
	WINDOWS       = Arch{"%s/clients/windows/%s-amd64.gz", "Download %s for Windows x86_64"}
	MAC_ARM       = Arch{"%s/clients/darwin/%s-arm64.gz", "Download %s for Mac arm64"}
	MAC_X86       = Arch{"%s/clients/darwin/%s-amd64.gz", "Download %s for Mac x86_64"}
	LINUX_S390X   = Arch{"%s/clients/linux/%s-s390x.gz", "Download %s for Linux s390x"}
	LINUX_PPC64LE = Arch{"%s/clients/linux/%s-ppc64le.gz", "Download %s for Linux ppc64le"}
	LINUX_ARM     = Arch{"%s/clients/linux/%s-arm64.gz", "Download %s for Linux arm64"}
	LINUX_X86     = Arch{"%s/clients/linux/%s-amd64.gz", "Download %s for Linux x86_64"}

	ALL_ARCHS = []Arch{WINDOWS, MAC_ARM, MAC_X86, LINUX_S390X, LINUX_PPC64LE, LINUX_ARM, LINUX_X86}
)

var (
	CliHostName string
)

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
		kubernetes.EnsureIngressSpec(ctx, c.Client, *svc, rhtasv1alpha1.ExternalAccess{Host: CliHostName}, cliServerPortName),
		ensure.Optional(kubernetes.IsOpenShift(), kubernetes.EnsureIngressTLS()),
		ensure.Labels[*v1.Ingress](slices.Collect(maps.Keys(labels)), labels),
	); e != nil {
		return e
	}

	if kubernetes.IsOpenShift() {
		protocol := "http://"
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		for name, description := range map[string]string{
			"cosign":          "cosign is a CLI tool that allows you to manage sigstore artifacts.",
			"rekor-cli":       "rekor-cli is a CLI tool that allows you to interact with rekor server.",
			"gitsign":         "gitsign is a CLI tool that allows you to digitally sign and verify git commits.",
			"ec":              "Enterprise Contract CLI. Set of commands to help validate resources with the Enterprise Contract.",
			"fetch-tsa-certs": "fetch-tsa-certs is a cli used to configure the kms and tink signer types for Timestamp Authority.",
			"createtree":      "create-tree is a CLI tool which is used for creating new trees within trillian.",
			"updatetree":      "update-tree is a CLI tool which is used for managing existing tress within trillian.",
		} {
			if e := CreateResource[*consolev1.ConsoleCLIDownload](ctx, c.Client, c.Log,
				&consolev1.ConsoleCLIDownload{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
				}, c.ensureConsoleCLIDownload(protocol+ingress.Spec.Rules[0].Host, description, name, ALL_ARCHS...),
				ensure.Labels[*consolev1.ConsoleCLIDownload](slices.Collect(maps.Keys(labels)), labels),
			); e != nil {
				return e
			}
		}

		if e := CreateResource[*consolev1.ConsoleCLIDownload](ctx, c.Client, c.Log,
			&consolev1.ConsoleCLIDownload{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tuftool",
				},
			}, c.ensureConsoleCLIDownload(
				protocol+ingress.Spec.Rules[0].Host,
				"tuftool is a Rust command-line utility for generating and signing TUF repositories.",
				"tuftool", LINUX_X86),
			ensure.Labels[*consolev1.ConsoleCLIDownload](slices.Collect(maps.Keys(labels)), labels),
		); e != nil {
			return e
		}
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

func (c *Component) ensureConsoleCLIDownload(clientServerUrl, cliDescription, cliName string, supportedArchs ...Arch) func(*consolev1.ConsoleCLIDownload) error {
	return func(cd *consolev1.ConsoleCLIDownload) error {
		spec := &cd.Spec
		spec.Description = cliDescription
		spec.DisplayName = fmt.Sprintf("%s - Command Line Interface (CLI)", cliName)

		for _, arch := range supportedArchs {
			spec.Links = append(spec.Links, consolev1.CLIDownloadLink{
				Text: fmt.Sprintf(arch.descriptionTemplate, cliName),
				Href: fmt.Sprintf(arch.linkTemplate, clientServerUrl, cliName),
			})
		}
		return nil
	}

}
