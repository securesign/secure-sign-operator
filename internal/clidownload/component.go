package clidownload

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	cliServerNs        = "trusted-artifact-signer"
	cliServerName      = "cli-server"
	cliServerComponent = "client-server"
	sharedVolumeName   = "shared-data"
	cliBinaryPath      = "/opt/app-root/src/clients/*"
	cliWebServerPath   = "/var/www/html/clients/"
)

type Component struct {
	Client client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (c *Component) Start(ctx context.Context) error {
	var (
		err    error
		obj    []client.Object
		labels = map[string]string{
			"app.kubernetes.io/part-of": constants.AppName,
			kubernetes.ComponentLabel:   cliServerComponent,
		}
	)

	c.Log.Info("installing client server resources")

	ns := &core.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cliServerNs,
		},
	}

	obj = append(obj, ns)
	obj = append(obj, c.createDeployment(ns.Name, labels))
	svc := kubernetes.CreateService(ns.Name, cliServerName, 8080, labels)
	obj = append(obj, svc)
	ingress, err := kubernetes.CreateIngress(ctx, c.Client, *svc, rhtasv1alpha1.ExternalAccess{}, cliServerName, labels)
	if err != nil {
		c.Log.Error(err, "unable to prepare ingress resources")
		return err
	}
	obj = append(obj, ingress)

	if kubernetes.IsOpenShift() {
		protocol := "http://"
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		for name, description := range map[string]string{
			"cosign":    "cosign is a CLI tool that allows you to manage sigstore artifacts.",
			"rekor-cli": "rekor-cli is a CLI tool that allows you to interact with rekor server.",
			"gitsign":   "gitsign is a CLI tool that allows you to digitally sign and verify git commits.",
			"ec":        "Enterprise Contract CLI. Set of commands to help validate resources with the Enterprise Contract.",
		} {
			obj = append(obj, c.createConsoleCLIDownload(ns.Name, name, protocol+ingress.Spec.Rules[0].Host, description, labels))
		}
	}

	for _, o := range obj {

		err = c.replaceResource(ctx, o)
		if err != nil {
			c.Log.Error(err, "failed CreateOrUpdate resource", "namespace", o.GetNamespace(), "name", o.GetName())
			return err
		}
		c.Log.V(1).Info("CreateOrUpdate", "name", o.GetName(), "namespace", o.GetNamespace())
	}
	return nil
}

func (c *Component) createDeployment(namespace string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)

	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cliServerName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: apps.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: core.PodSpec{
					Volumes: []core.Volume{
						{
							Name: sharedVolumeName,
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
					},
					InitContainers: []core.Container{
						{
							Name:    "init-shared-data-cg",
							Image:   constants.ClientServerImage_cg,
							Command: []string{"sh", "-c", fmt.Sprintf("cp -r %s %s", cliBinaryPath, cliWebServerPath)},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      sharedVolumeName,
									MountPath: cliWebServerPath,
								},
							},
						},
						{
							Name:    "init-shared-data-re",
							Image:   constants.ClientServerImage_re,
							Command: []string{"sh", "-c", fmt.Sprintf("cp -r %s %s", cliBinaryPath, cliWebServerPath)},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      sharedVolumeName,
									MountPath: cliWebServerPath,
								},
							},
						},
					},
					Containers: []core.Container{
						{
							Name:            cliServerName,
							Image:           constants.ClientServerImage,
							ImagePullPolicy: core.PullAlways,
							Ports: []core.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      core.ProtocolTCP,
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      sharedVolumeName,
									MountPath: cliWebServerPath,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *Component) createConsoleCLIDownload(namespace, name, clientServerUrl, description string, labels map[string]string) *consolev1.ConsoleCLIDownload {
	return &consolev1.ConsoleCLIDownload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: consolev1.ConsoleCLIDownloadSpec{
			Description: description,
			DisplayName: fmt.Sprintf("%s - Command Line Interface (CLI)", name),
			Links: []consolev1.CLIDownloadLink{
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-ppc64le.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux ppc64le", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-s390x.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux s390x", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/windows/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Windows x86_64", name),
				},
			},
		},
	}
}

func (c *Component) replaceResource(ctx context.Context, res client.Object) error {
	err := c.Client.Create(ctx, res)
	if err != nil && apierrors.IsAlreadyExists(err) {
		existing, ok := res.DeepCopyObject().(client.Object)
		if !ok {
			return fmt.Errorf("type assertion failed: %v", res.DeepCopyObject())
		}
		err = c.Client.Get(ctx, client.ObjectKeyFromObject(existing), existing)
		if err != nil {
			return err
		}
		res.SetResourceVersion(existing.GetResourceVersion())
		err = c.Client.Update(ctx, res)
	}
	if err != nil {
		return fmt.Errorf("could not create or replace %s: %w"+res.GetObjectKind().GroupVersionKind().String(), err)
	}
	return nil
}
