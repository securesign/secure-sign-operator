package deployment

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/tls"
	v1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const name = "dp"

func TestEnsureTrustedCA(t *testing.T) {
	gomega.RegisterTestingT(t)
	t.Run("update existing object", func(t *testing.T) {

		ctx := context.TODO()
		c := testAction.FakeClientBuilder().
			WithObjects(&v1.Deployment{
				ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1.DeploymentSpec{
					Template: core.PodTemplateSpec{
						Spec: core.PodSpec{
							Containers: []core.Container{
								{
									Name: name, Image: "test",
									Env: []core.EnvVar{
										{Name: "NAME", Value: "VALUE"},
									},
									VolumeMounts: []core.VolumeMount{
										{
											MountPath: "path",
											Name:      "mount",
										},
									},
								},
							},
							Volumes: []core.Volume{
								{
									Name: "mount",
								},
							},
						},
					},
				},
			}).
			Build()

		result, err := kubernetes.CreateOrUpdate(ctx, c,
			&v1.Deployment{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
			TrustedCA(&v1alpha1.LocalObjectReference{Name: "test"}, name),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(result).To(gomega.Equal(controllerutil.OperationResultUpdated))

		existing := &v1.Deployment{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Name).To(gomega.Equal("NAME"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Value).To(gomega.Equal("VALUE"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[1].Name).To(gomega.Equal("SSL_CERT_DIR"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[1].Value).To(gomega.Equal("/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(gomega.Equal("mount"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal("path"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[1].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[1].MountPath).To(gomega.Equal("/var/run/configs/tas/ca-trust"))

		gomega.Expect(existing.Spec.Template.Spec.Volumes).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Name).To(gomega.Equal("mount"))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Projected.Sources).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[1].Projected.Sources[0].ConfigMap.Name).To(gomega.Equal("test"))

	})
}

func TestEnsureTLS(t *testing.T) {
	gomega.RegisterTestingT(t)
	t.Run("update existing object", func(t *testing.T) {

		ctx := context.TODO()
		c := testAction.FakeClientBuilder().
			WithObjects(&v1.Deployment{
				ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1.DeploymentSpec{
					Template: core.PodTemplateSpec{
						Spec: core.PodSpec{
							Containers: []core.Container{
								{Name: name, Image: "test"},
								{Name: "doNotUpdate", Image: "test"},
							},
						},
					},
				},
			}).
			Build()

		result, err := kubernetes.CreateOrUpdate(ctx, c,
			&v1.Deployment{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
			TLS(v1alpha1.TLS{
				PrivateKeyRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "testSecret",
					},
					Key: "key",
				},
				CertRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "testSecret",
					},
					Key: "cert",
				},
			}, name),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(result).To(gomega.Equal(controllerutil.OperationResultUpdated))

		existing := &v1.Deployment{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(gomega.Equal(tls.TLSVolumeName))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal("/var/run/secrets/tas"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[1].VolumeMounts).To(gomega.BeEmpty())

		gomega.Expect(existing.Spec.Template.Spec.Volumes).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Name).To(gomega.Equal(tls.TLSVolumeName))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources).To(gomega.HaveLen(2))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources).To(gomega.ContainElements(
			gomega.And(
				gomega.WithTransform(func(s core.VolumeProjection) string {
					return s.Secret.Name
				}, gomega.Equal("testSecret")),
				gomega.WithTransform(func(s core.VolumeProjection) string {
					return s.Secret.Items[0].Key
				}, gomega.Equal("key")),
			),
			gomega.And(
				gomega.WithTransform(func(s core.VolumeProjection) string {
					return s.Secret.Name
				}, gomega.Equal("testSecret")),
				gomega.WithTransform(func(s core.VolumeProjection) string {
					return s.Secret.Items[0].Key
				}, gomega.Equal("cert")),
			),
		))

	})
}

func TestPodRequirements(t *testing.T) {
	type args struct {
		requirements  v1alpha1.PodRequirements
		containerName string
	}
	tests := []struct {
		name   string
		args   args
		verify func(gomega.Gomega, *v1.Deployment)
	}{
		{
			name: "empty requirements",
			args: args{
				requirements:  v1alpha1.PodRequirements{},
				containerName: "container",
			},
			verify: func(g gomega.Gomega, deployment *v1.Deployment) {
				g.Expect(deployment.Spec.Replicas).To(gomega.BeNil())
				g.Expect(deployment.Spec.Template.Spec.Affinity).To(gomega.BeNil())
				g.Expect(deployment.Spec.Template.Spec.Tolerations).To(gomega.BeEmpty())

				g.Expect(deployment.Spec.Template.Spec.Containers).To(gomega.HaveLen(1))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(gomega.Equal("container"))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources).To(gomega.BeZero())
			},
		},
		{
			name: "affinity",
			args: args{
				requirements: v1alpha1.PodRequirements{
					Affinity: &core.Affinity{},
				},
				containerName: "container",
			},
			verify: func(g gomega.Gomega, deployment *v1.Deployment) {
				g.Expect(deployment.Spec.Template.Spec.Affinity).ToNot(gomega.BeNil())
			},
		},
		{
			name: "resources",
			args: args{
				requirements: v1alpha1.PodRequirements{
					Resources: &core.ResourceRequirements{
						Limits: core.ResourceList{
							core.ResourceCPU: resource.MustParse("100m"),
						},
					},
				},
				containerName: "container",
			},
			verify: func(g gomega.Gomega, deployment *v1.Deployment) {
				g.Expect(deployment.Spec.Template.Spec.Containers).To(gomega.HaveLen(1))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(gomega.Equal("container"))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Resources).ToNot(gomega.BeZero())
				g.Expect(*deployment.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu()).To(gomega.Equal(resource.MustParse("100m")))
			},
		},
		{
			name: "tolerations",
			args: args{
				requirements: v1alpha1.PodRequirements{
					Tolerations: []core.Toleration{
						{
							Key:      "key",
							Operator: core.TolerationOpExists,
						},
					},
				},
			},
			verify: func(g gomega.Gomega, deployment *v1.Deployment) {
				g.Expect(deployment.Spec.Template.Spec.Tolerations).To(gomega.HaveLen(1))
				g.Expect(deployment.Spec.Template.Spec.Tolerations[0].Key).To(gomega.Equal("key"))
				g.Expect(deployment.Spec.Template.Spec.Tolerations[0].Operator).To(gomega.Equal(core.TolerationOpExists))
			},
		},
		{
			name: "replicas",
			args: args{
				requirements: v1alpha1.PodRequirements{
					Replicas: ptr.To(int32(10)),
				},
			},
			verify: func(g gomega.Gomega, deployment *v1.Deployment) {
				g.Expect(deployment.Spec.Replicas).To(gomega.Equal(ptr.To(int32(10))))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			dp := &v1.Deployment{}
			fn := PodRequirements(tt.args.requirements, tt.args.containerName)
			if got := fn(dp); got != nil {
				t.Errorf("PodRequirements() = %v", got)
			}
			tt.verify(g, dp)
		})
	}
}
