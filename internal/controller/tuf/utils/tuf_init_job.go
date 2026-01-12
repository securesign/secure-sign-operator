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
		var (
			protocol, uri string
		)
		if tls.UseTlsClient(instance) {
			protocol = "https"
		} else {
			protocol = "http"
		}

		// prepare args
		args := []string{"--export-keys", instance.Spec.RootKeySecretRef.Name}
		for _, key := range instance.Spec.Keys {
			switch key.Name {
			case "rekor.pub":
				if instance.Spec.Rekor.Address != "" {
					uri = instance.Spec.Rekor.Address
					if instance.Spec.Rekor.Port != nil {
						uri = fmt.Sprintf("%s:%d", uri, *instance.Spec.Rekor.Port)
					}
				} else {
					uri = fmt.Sprintf("%s://%s.%s.svc", protocol, rekor.ServerDeploymentName, instance.Namespace)
				}
				args = append(args, "--rekor-key", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--rekor-uri", uri)
			case "ctfe.pub":
				if instance.Spec.Ctlog.Prefix == "" {
					return futils.ErrCtlogPrefixNotSpecified
				}

				if instance.Spec.Ctlog.Address != "" {
					uri = instance.Spec.Ctlog.Address
					if instance.Spec.Ctlog.Port != nil {
						uri = fmt.Sprintf("%s:%d", uri, *instance.Spec.Ctlog.Port)
					}
					uri = fmt.Sprintf("%s/%s", uri, instance.Spec.Ctlog.Prefix)
				} else {
					uri = fmt.Sprintf("%s://%s.%s.svc/%s", protocol, ctlog.DeploymentName, instance.Namespace, instance.Spec.Ctlog.Prefix)
				}

				args = append(args, "--ctlog-key", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--ctlog-uri", uri)
			case "fulcio_v1.crt.pem":
				if instance.Spec.Fulcio.Address != "" {
					uri = instance.Spec.Fulcio.Address
					if instance.Spec.Fulcio.Port != nil {
						uri = fmt.Sprintf("%s:%d", uri, *instance.Spec.Fulcio.Port)
					}
				} else {
					uri = fmt.Sprintf("%s://%s.%s.svc", protocol, fulcio.DeploymentName, instance.Namespace)
				}
				args = append(args, "--fulcio-cert", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--fulcio-uri", uri)
			case "tsa.certchain.pem":
				if instance.Spec.Tsa.Address != "" {
					uri = instance.Spec.Tsa.Address
					if instance.Spec.Tsa.Port != nil {
						uri = fmt.Sprintf("%s:%d", uri, *instance.Spec.Tsa.Port)
					}
				} else {
					uri = fmt.Sprintf("%s://%s.%s.svc", protocol, tsa.DeploymentName, instance.Namespace)
				}
				args = append(args, "--tsa-cert", filepath.Join(secretsMonthPath, key.Name))
				args = append(args, "--tsa-uri", uri)
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
