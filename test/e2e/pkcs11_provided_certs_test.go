//go:build integration

package e2e

import (
	"encoding/json"
	"time"

	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Securesign install with PKCS#11 reference mode", Ordered, func() {
	cli, _ := support.CreateClient()

	const (
		credSecretName = "my-hsm-credentials"
		confSecretName = "my-hsm-pkcs11-config"
		softhsmCMName  = "my-softhsm-config"
	)

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
			securesign.WithPKCS11RefCerts(credSecretName, confSecretName, softhsmCMName),
			securesign.WithManagedDatabase(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with pre-created PKCS#11 secrets", func() {
		BeforeAll(func(ctx SpecContext) {
			confJSON, err := json.Marshal(map[string]string{
				"Path":       "/var/lib/hsm/lib/libsofthsm2.so",
				"TokenLabel": "fulcio",
				"Pin":        "test-pin-2324",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(cli.Create(ctx, &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: credSecretName, Namespace: namespace.Name},
				Data:       map[string][]byte{"pin": []byte("test-pin-2324")},
			})).To(Succeed())

			Expect(cli.Create(ctx, &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: confSecretName, Namespace: namespace.Name},
				Data:       map[string][]byte{"crypto11.conf": confJSON},
			})).To(Succeed())

			Expect(cli.Create(ctx, &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: softhsmCMName, Namespace: namespace.Name},
				Data: map[string]string{
					"softhsm2.conf": "directories.tokendir = /var/lib/hsm/tokens\nobjectstore.backend = file\nlog.level = INFO\n",
				},
			})).To(Succeed())

			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Operator did not create additional credentials secrets", func(ctx SpecContext) {
			list := &v1.SecretList{}
			Expect(cli.List(ctx, list,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{actions.PKCS11CredLabel: "pin"})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "operator should not create a credentials secret when credentialsRef is provided")
		})

		It("Operator did not create additional pkcs11 config secrets", func(ctx SpecContext) {
			list := &v1.SecretList{}
			Expect(cli.List(ctx, list,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{actions.PKCS11ConfLabel: "crypto11.conf"})).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "operator should not create a config secret when pkcs11ConfigRef is provided")
		})

		It("Status references the user-provided secrets", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			Expect(f).NotTo(BeNil())
			Expect(f.Status.Certificate).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11.CredentialsRef).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11.CredentialsRef.Name).To(Equal(credSecretName))
			Expect(f.Status.Certificate.PKCS11.PKCS11ConfigRef).NotTo(BeNil())
			Expect(f.Status.Certificate.PKCS11.PKCS11ConfigRef.Name).To(Equal(confSecretName))
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

		It("Fulcio server uses PKCS#11 CA args", func(ctx SpecContext) {
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
		})

		It("HSM root CA secret was created and labeled for discovery", func(ctx SpecContext) {
			var caRef *v1alpha1.SecretKeySelector
			Eventually(func() *v1alpha1.SecretKeySelector {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				if f == nil || f.Status.Certificate == nil {
					return nil
				}
				caRef = f.Status.Certificate.CARef
				return caRef
			}).WithTimeout(3 * time.Minute).ShouldNot(BeNil())

			list := &v1.SecretList{}
			Expect(cli.List(ctx, list,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{labels.LabelNamespace + "/fulcio_v1.crt.pem": "cert"})).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			Expect(list.Items[0].Name).To(Equal(caRef.Name))
		})

		It("Init container volume uses user-provided ConfigMap", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			var found bool
			for _, vol := range server.Spec.Volumes {
				if vol.ConfigMap != nil && vol.ConfigMap.Name == softhsmCMName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "pod should mount user-provided ConfigMap %s", softhsmCMName)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
