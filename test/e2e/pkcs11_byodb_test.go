//go:build integration

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("PKCS#11 install with BYODB", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(func() {
		SetDefaultEventuallyTimeout(6 * time.Minute)
	})

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		dsn := "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp(my-mysql.$(NAMESPACE).svc:3300)/$(MYSQL_DB)"
		if testSupportKubernetes.IsRemoteClusterOpenshift() {
			dsn += "?tls=true"
		}

		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithPKCS11Certs(),
			securesign.WithExternalDatabase(dbAuth),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
			func(v *v1alpha1.Securesign) {
				v.Spec.Rekor.Auth = &v1alpha1.Auth{
					Env: []v1.EnvVar{
						{
							Name: "MYSQL_USER",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-user",
							}},
						},
						{
							Name: "MYSQL_PASSWORD",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-password",
							}},
						},
						{
							Name: "MYSQL_DB",
							ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{
									Name: dbAuth,
								},
								Key: "mysql-database",
							}},
						},
						{
							Name: "NAMESPACE",
							ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{
								APIVersion: "v1",
								FieldPath:  "metadata.namespace",
							}},
						},
					},
				}
				v.Spec.Rekor.BackFillRedis = v1alpha1.BackFillRedis{
					Enabled:  ptr.To(true),
					Schedule: "* * * * *",
				}
				v.Spec.Rekor.SearchIndex = v1alpha1.SearchIndex{
					Create:   ptr.To(false),
					Provider: "mysql",
					Url:      dsn,
				}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with PKCS#11 and external database", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(createDB(ctx, cli, namespace.Name, dbAuth)).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, false)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
