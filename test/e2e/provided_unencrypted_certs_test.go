//go:build integration

package e2e

import (
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Securesign install with provided unencrypted certs", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *rhtasv1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithProvidedUnencryptedCerts(),
			func(v *rhtasv1.Securesign) {
				v.Spec.Tuf.Keys = []rhtasv1.TufKey{
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
					{
						Name: "rekor.pub",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-rekor-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-ctlog-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "certificateChain",
						},
					},
				}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with unencrypted certificates", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret", false))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret", false))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret", false))).To(Succeed())
			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "test-tsa-secret", false))).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("Fulcio is running with mounted certs", func(ctx SpecContext) {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			sp := []v1.SecretProjection{}
			for _, volume := range server.Spec.Volumes {
				if volume.Name == "fulcio-cert" {
					for _, source := range volume.Projected.Sources {
						sp = append(sp, *source.Secret)
					}
				}
			}

			Expect(sp).To(
				ContainElement(
					WithTransform(func(sp v1.SecretProjection) string {
						return sp.Name
					}, Equal("my-fulcio-secret")),
				))

		})

		It("Rekor is running with mounted certs", func(ctx SpecContext) {
			rekor.Verify(ctx, cli, namespace.Name, s.Name, true)
			server := rekor.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).NotTo(BeNil())
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.Secret != nil {
							return volume.Secret.SecretName
						}
						return ""
					}, Equal("my-rekor-secret")),
				))

		})

		It("tsa is running with mounted certs", func(ctx SpecContext) {
			tsa.Verify(ctx, cli, namespace.Name, s.Name)
			tsa := tsa.GetServerPod(ctx, cli, namespace.Name)()
			Expect(tsa).NotTo(BeNil())
			Expect(tsa.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.Secret != nil {
							return volume.Secret.SecretName
						}
						return ""
					}, Equal("test-tsa-secret")),
				))
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
