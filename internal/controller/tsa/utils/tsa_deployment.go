package tsaUtils

import (
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateTimestampAuthorityDeployment(instance *v1alpha1.TimestampAuthority, name string, sa string, labels map[string]string) *apps.Deployment {
	replicas := int32(1)
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
					Containers: []core.Container{
						{
							Name:  name,
							Image: constants.TimestampAuthorityImage,
							Ports: []core.ContainerPort{
								{
									Protocol:      core.ProtocolTCP,
									ContainerPort: 3000,
								},
							},
							Command: []string{
								"timestamp-server",
								"serve",
								"--host=0.0.0.0",
								"--port=3000",
								"--timestamp-signer=memory",
							},
						},
					},
				},
			},
		},
	}
}
