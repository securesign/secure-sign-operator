package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	ctlog "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	rekor "github.com/securesign/operator/internal/controller/rekor/actions"
	tsa "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

const (
	secretsMonthPath = "/var/run/tuf-secrets"
	targetMonthPath  = "/var/run/target"
)

type ServicesURIs struct {
	Ctlog  string
	Fulcio string
	Rekor  string
	Tsa    string
}

func EnsureTufInitJob(instance *rhtasv1alpha1.Tuf, sa string, labels map[string]string) func(*batchv1.Job) error {
	return func(job *batchv1.Job) error {

		// prepare args
		servicesURIs, err := resolveServicesUrls(instance)
		if err != nil {
			return fmt.Errorf("could not resolve services urls: %w", err)
		}
		args := []string{"--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case "rekor.pub":
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--rekor-uri", servicesURIs.Rekor)
			case "ctfe.pub":
				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--ctlog-uri", servicesURIs.Ctlog)
			case "fulcio_v1.crt.pem":
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--fulcio-uri", servicesURIs.Fulcio)
			case "tsa.certchain.pem":
				args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--tsa-uri", servicesURIs.Tsa)
			}
		}
		args = append(args, targetMonthPath)

		jobSpec := &job.Spec
		jobSpec.Parallelism = ptr.To[int32](1)
		jobSpec.Completions = ptr.To[int32](1)
		jobSpec.BackoffLimit = ptr.To(int32(0))
		jobSpec.Template.Labels = labels

		templateSpec := &jobSpec.Template.Spec
		templateSpec.ServiceAccountName = sa
		templateSpec.RestartPolicy = v1.RestartPolicyNever

		// initialize volumes
		secretsVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, "tuf-secrets")
		secretsVolume.VolumeSource = v1.VolumeSource{
			Projected: secretsVolumeProjection(instance.Status.Keys),
		}

		repositoryVolume := kubernetes.FindVolumeByNameOrCreate(templateSpec, constants.VolumeName)
		repositoryVolume.VolumeSource = v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Status.PvcName,
			},
		}
		// init containers
		container := kubernetes.FindContainerByNameOrCreate(templateSpec, "tuf-init")
		container.Image = images.Registry.Get(images.Tuf)
		env := kubernetes.FindEnvByNameOrCreate(container, "NAMESPACE")
		env.Value = instance.Namespace
		container.Command = []string{"/bin/bash", "-c"}
		container.Args = []string{
			fmt.Sprintf("tuf-repo-init.sh %s; ", strings.Join(args, " ")) +
				"exit_code=$?; " +
				"if [ $exit_code -eq 2 ]; then exit 0; else exit $exit_code; fi",
		}
		container.VolumeMounts = []v1.VolumeMount{
			{
				Name:      "tuf-secrets",
				MountPath: secretsMonthPath,
			},
			{
				Name:      "repository",
				MountPath: targetMonthPath,
				ReadOnly:  false,
			},
		}

		return nil
	}
}

func resolveServicesUrls(instance *rhtasv1alpha1.Tuf) (ServicesURIs, error) {
	uris := ServicesURIs{}
	var (
		protocol string
	)
	if tls.UseTlsClient(instance) {
		protocol = "https"
	} else {
		protocol = "http"
	}

	// Ctlog
	if instance.Spec.Ctlog.Prefix == "" {
		return ServicesURIs{}, futils.ErrCtlogPrefixNotSpecified
	}

	if instance.Spec.Ctlog.Address != "" {
		url := instance.Spec.Ctlog.Address
		if instance.Spec.Ctlog.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Ctlog.Port)
		}
		uris.Ctlog = fmt.Sprintf("%s/%s", url, instance.Spec.Ctlog.Prefix)
	} else {
		uris.Ctlog = fmt.Sprintf("%s://%s.%s.svc/%s", protocol, ctlog.DeploymentName, instance.Namespace, instance.Spec.Ctlog.Prefix)
	}

	// Fulcio
	if instance.Spec.Fulcio.Address != "" {
		url := instance.Spec.Fulcio.Address
		if instance.Spec.Fulcio.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Fulcio.Port)
		}
		uris.Fulcio = url
	} else {
		uris.Fulcio = fmt.Sprintf("%s://%s.%s.svc", protocol, fulcio.DeploymentName, instance.Namespace)
	}

	// Rekor
	if instance.Spec.Rekor.Address != "" {
		url := instance.Spec.Rekor.Address
		if instance.Spec.Rekor.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Rekor.Port)
		}
		uris.Rekor = url
	} else {
		uris.Rekor = fmt.Sprintf("%s://%s.%s.svc", protocol, rekor.ServerDeploymentName, instance.Namespace)
	}

	// TSA
	if instance.Spec.Tsa.Address != "" {
		url := instance.Spec.Tsa.Address
		if instance.Spec.Tsa.Port != nil {
			url = fmt.Sprintf("%s:%d", url, *instance.Spec.Tsa.Port)
		}
		uris.Tsa = url
	} else {
		uris.Tsa = fmt.Sprintf("%s://%s.%s.svc", protocol, tsa.DeploymentName, instance.Namespace)
	}

	return uris, nil
}
