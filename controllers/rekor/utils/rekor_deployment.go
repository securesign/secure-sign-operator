package utils

import (
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRekorDeployment(instance *v1alpha1.Rekor, dpName string, labels map[string]string, serviceAccountName string) *apps.Deployment {
	if serviceAccountName == "" {
		serviceAccountName = constants.ServiceAccountName
	}
	env := make([]core.EnvVar, 0)
	appArgs := []string{
		"serve",
		"--trillian_log_server.address=trillian-logserver." + instance.Namespace + ".svc",
		"--trillian_log_server.port=8091",
		"--trillian_log_server.sharding_config=/sharding/sharding-config.yaml",
		"--redis_server.address=rekor-redis",
		"--redis_server.port=6379",
		"--rekor_server.address=0.0.0.0",
		"--enable_retrieve_api=true",
		fmt.Sprintf("--trillian_log_server.tlog_id=%d", *instance.Status.TreeID),
		"--enable_attestation_storage",
		"--attestation_storage_bucket=file:///var/run/attestations",
	}
	volumes := []core.Volume{
		{
			Name: "rekor-sharding-config",
			VolumeSource: core.VolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: "rekor-sharding-config",
					},
				},
			},
		},
		{
			Name: "storage",
			VolumeSource: core.VolumeSource{
				PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
					ClaimName: instance.Spec.PvcName,
				},
			},
		},
	}
	volumeMounts := []core.VolumeMount{
		{
			Name:      "rekor-sharding-config",
			MountPath: "/sharding",
		},
		{
			Name:      "storage",
			MountPath: "/var/run/attestations",
		},
	}

	// KMS memory
	if instance.Spec.Signer.KMS == "memory" {
		appArgs = append(appArgs, "--rekor_server.signer=memory")
	}

	// KMS secret
	if instance.Spec.Signer.KMS == "secret" || instance.Spec.Signer.KMS == "" {
		svsPrivate := &core.SecretVolumeSource{
			SecretName: instance.Spec.Signer.KeyRef.Name,
			Items: []core.KeyToPath{
				{
					Key:  instance.Spec.Signer.KeyRef.Key,
					Path: "private",
				},
			},
		}

		appArgs = append(appArgs, "--rekor_server.signer=/key/private")

		volumes = append(volumes, core.Volume{
			Name: "rekor-private-key-volume",
			VolumeSource: core.VolumeSource{
				Secret: svsPrivate,
			},
		})

		volumeMounts = append(volumeMounts, core.VolumeMount{
			Name:      "rekor-private-key-volume",
			MountPath: "/key",
			ReadOnly:  true,
		})

		// Add signer password
		if instance.Spec.Signer.PasswordRef != nil {
			appArgs = append(appArgs, "--rekor_server.signer-passwd=$(SIGNER_PASSWORD)")
			env = append(env, core.EnvVar{
				Name: "SIGNER_PASSWORD",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						Key: instance.Spec.Signer.PasswordRef.Key,
						LocalObjectReference: core.LocalObjectReference{
							Name: instance.Spec.Signer.PasswordRef.Name,
						},
					},
				},
			})
		}
	}
	//TODO mount additional ENV variables and secrets to enable cloud KMS service

	replicas := int32(1)
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpName,
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
					ServiceAccountName: serviceAccountName,
					Volumes:            volumes,
					Containers: []core.Container{
						{
							Name: dpName,
							// TODO add probe
							//LivenessProbe: &core.Probe{
							//	},
							//	InitialDelaySeconds: 30,
							//	TimeoutSeconds:      1,
							//	PeriodSeconds:       10,
							//	SuccessThreshold:    1,
							//	FailureThreshold:    3,
							//},
							Image: constants.RekorServerImage,
							Ports: []core.ContainerPort{
								{
									ContainerPort: 3000,
									Name:          "rekor-server",
								},
								{
									ContainerPort: 2112,
									Protocol:      "TCP",
								},
							},
							Env:          env,
							Args:         appArgs,
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
		},
	}
}
