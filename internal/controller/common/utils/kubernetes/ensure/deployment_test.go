package ensure

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/apps/v1"
	v3 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const name = "dp"

func TestEnsureTrustedCAFromAnnotations(t *testing.T) {
	gomega.RegisterTestingT(t)
	t.Run("update existing object", func(t *testing.T) {

		ctx := context.TODO()
		c := testAction.FakeClientBuilder().
			WithObjects(&v1.Deployment{
				ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
				Spec: v1.DeploymentSpec{
					Template: v3.PodTemplateSpec{
						Spec: v3.PodSpec{
							Containers: []v3.Container{
								{Name: name, Image: "test"},
							},
						},
					},
				},
			}).
			Build()

		result, err := kubernetes.CreateOrUpdate(ctx, c,
			&v1.Deployment{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
			TrustedCA(utils.TrustedCAAnnotationToReference(map[string]string{annotations.TrustedCA: "test"})),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Expect(result).To(gomega.Equal(controllerutil.OperationResultUpdated))

		existing := &v1.Deployment{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, existing)).To(gomega.Succeed())
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Name).To(gomega.Equal("SSL_CERT_DIR"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].Env[0].Value).To(gomega.Equal("/var/run/configs/tas/ca-trust:/var/run/secrets/kubernetes.io/serviceaccount"))

		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(gomega.Equal("/var/run/configs/tas/ca-trust"))

		gomega.Expect(existing.Spec.Template.Spec.Volumes).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Name).To(gomega.Equal("ca-trust"))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources).To(gomega.HaveLen(1))
		gomega.Expect(existing.Spec.Template.Spec.Volumes[0].Projected.Sources[0].ConfigMap.Name).To(gomega.Equal("test"))

	})
}
