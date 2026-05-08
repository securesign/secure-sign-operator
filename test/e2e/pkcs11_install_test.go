//go:build integration

package e2e

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"

	"github.com/securesign/operator/test/e2e/support/tas"

	"github.com/securesign/operator/test/e2e/support/tas/fulcio"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Securesign install with PKCS#11 CA", Ordered, func() {
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
		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithPKCS11Certs(),
			securesign.WithManagedDatabase(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
			securesign.WithMonitoring(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with PKCS#11 Fulcio CA", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Fulcio has PKCS11ConfigAvailable condition", func(ctx SpecContext) {
			Eventually(func() bool {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				if f == nil {
					return false
				}
				return meta.IsStatusConditionTrue(f.GetConditions(), actions.PKCS11ConfigCondition)
			}).WithTimeout(3 * time.Minute).Should(BeTrue())
		})

		It("Fulcio pod has PKCS#11 init containers", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			initNames := make([]string, 0, len(server.Spec.InitContainers))
			for _, c := range server.Spec.InitContainers {
				initNames = append(initNames, c.Name)
			}
			Expect(initNames).To(ContainElements("hsm-init", "hsm-lib-export", "fulcio-createca"))
		})

		It("All init containers completed successfully", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			for _, status := range server.Status.InitContainerStatuses {
				Expect(status.State.Terminated).NotTo(BeNil(), "init container %s should be terminated", status.Name)
				Expect(status.State.Terminated.ExitCode).To(Equal(int32(0)), "init container %s should exit 0", status.Name)
			}
		})

		It("Operator created PKCS#11 secrets", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			Expect(f).NotTo(BeNil())
			Expect(f.Status.Certificate).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11.CredentialsRef).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11.PKCS11ConfigRef).NotTo(BeNil())
		})

		It("Fulcio server uses PKCS#11 CA args and not file-CA args", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			var fulcioContainer *v1.Container
			for i := range server.Spec.Containers {
				if server.Spec.Containers[i].Name == "fulcio-server" {
					fulcioContainer = &server.Spec.Containers[i]
					break
				}
			}
			Expect(fulcioContainer).NotTo(BeNil())
			Expect(fulcioContainer.Args).To(ContainElements("--ca=pkcs11ca"))
			Expect(fulcioContainer.Args).NotTo(ContainElements("--ca=fileca"))
			Expect(fulcioContainer.Args).NotTo(ContainElements("--fileca-key"))
		})

		It("File-CA fields are absent from Fulcio status", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			Expect(f).NotTo(BeNil())
			Expect(f.Status.Certificate.PrivateKeyRef).To(BeNil())
			Expect(f.Status.Certificate.PrivateKeyPasswordRef).To(BeNil())
		})

		It("Root CA certificate subject matches PKCS#11 rootCA config", func(ctx SpecContext) {
			list := &v1.SecretList{}
			Expect(cli.List(ctx, list,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{labels.LabelNamespace + "/fulcio_v1.crt.pem": "cert"})).To(Succeed())
			Expect(list.Items).NotTo(BeEmpty())

			certPEM := list.Items[0].Data["cert"]
			Expect(certPEM).NotTo(BeEmpty())

			block, _ := pem.Decode(certPEM)
			Expect(block).NotTo(BeNil(), "failed to decode PEM")

			cert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			Expect(cert.Subject.Organization).To(ContainElement("RHTAS"))
			Expect(cert.IsCA).To(BeTrue())
			Expect(cert.Issuer.Organization).To(ContainElement("RHTAS"))
			GinkgoWriter.Printf("Root CA subject: %s\n", cert.Subject)
		})

		It("HSM root CA secret was created and labeled for CTLog/TUF discovery", func(ctx SpecContext) {
			var caRef *v1alpha1.SecretKeySelector
			Eventually(func() *v1alpha1.SecretKeySelector {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				if f == nil || f.Status.Certificate == nil {
					return nil
				}
				caRef = f.Status.Certificate.CARef
				return caRef
			}).WithTimeout(3 * time.Minute).ShouldNot(BeNil())

			secret := &v1.Secret{}
			Expect(cli.Get(ctx, types.NamespacedName{
				Namespace: namespace.Name,
				Name:      caRef.Name,
			}, secret)).To(Succeed())
			Expect(secret.Labels).To(HaveKeyWithValue(labels.LabelNamespace+"/fulcio_v1.crt.pem", "cert"))
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
