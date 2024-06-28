package tsaUtils

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTimestampAuthorityDeployment(instance *v1alpha1.TimestampAuthority, name string, sa string, labels map[string]string) *apps.Deployment {
	env := make([]core.EnvVar, 0)
	volumes := make([]core.Volume, 0)
	volumeMounts := make([]core.VolumeMount, 0)
	replicas := int32(1)

	appArgs := []string{
		"timestamp-server",
		"serve",
		"--host=0.0.0.0",
		"--port=3000",
		"--certificate-chain-path=/etc/secrets/cert_chain/certificate-chain.pem",
	}

	volumes = append(volumes, core.Volume{
		Name: "tsa-cert-chain",
		VolumeSource: core.VolumeSource{
			Secret: &core.SecretVolumeSource{
				SecretName: instance.Status.Signer.CertificateChain.CertificateChainRef.Name,
				Items: []core.KeyToPath{
					{
						Key:  instance.Status.Signer.CertificateChain.CertificateChainRef.Key,
						Path: "certificate-chain.pem",
					},
				},
			},
		},
	})

	volumeMounts = append(volumeMounts, core.VolumeMount{
		Name:      "tsa-cert-chain",
		MountPath: "/etc/secrets/cert_chain",
		ReadOnly:  true,
	})

	switch instance.Spec.Signer.Type {
	case "file":
		volumes = append(volumes, core.Volume{
			Name: "tsa-file-signer-config",
			VolumeSource: core.VolumeSource{
				Secret: &core.SecretVolumeSource{
					SecretName: instance.Status.Signer.FileSigner.PrivateKeyRef.Name,
					Items: []core.KeyToPath{
						{
							Key:  instance.Status.Signer.FileSigner.PrivateKeyRef.Key,
							Path: "private_key.pem",
						},
					},
				},
			},
		})

		volumeMounts = append(volumeMounts, core.VolumeMount{
			Name:      "tsa-file-signer-config",
			MountPath: "/etc/secrets/keys",
			ReadOnly:  true,
		})

		if instance.Status.Signer.FileSigner.PasswordRef != nil {
			env = append(env, core.EnvVar{
				Name: "SIGNER_PASSWORD",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: instance.Status.Signer.FileSigner.PasswordRef.Name,
						},
						Key: instance.Status.Signer.FileSigner.PasswordRef.Key,
					},
				},
			})
		}

		appArgs = append(appArgs,
			"--timestamp-signer=file",
			"--file-signer-key-path=/etc/secrets/keys/private_key.pem",
			"--file-signer-passwd=$(SIGNER_PASSWORD)",
		)
	}

	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
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
					ServiceAccountName: sa,
					Volumes:            volumes,
					Containers: []core.Container{
						{
							Name:         name,
							VolumeMounts: volumeMounts,
							Env:          env,
							Image:        constants.TimestampAuthorityImage,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 3000,
								},
							},
							Command: appArgs,
						},
					},
				},
			},
		},
	}
}
