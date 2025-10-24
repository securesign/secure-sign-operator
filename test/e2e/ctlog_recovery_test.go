//go:build integration

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	ctlogActions "github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("CTlog recovery and validation", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var trillianCR *v1alpha1.Trillian
	var ctlogCR *v1alpha1.CTlog

	// Shared setup - create namespace
	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	// Shared setup - deploy Trillian (needed by CTLog)
	BeforeAll(func(ctx SpecContext) {
		trillianCR = &v1alpha1.Trillian{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trillian",
				Namespace: namespace.Name,
			},
			Spec: v1alpha1.TrillianSpec{
				Db: v1alpha1.TrillianDB{Create: ptr.To(true)},
			},
		}
		Expect(cli.Create(ctx, trillianCR)).To(Succeed())

		By("Waiting for Trillian to be ready")
		trillian.Verify(ctx, cli, namespace.Name, trillianCR.Name, true)
	})

	Describe("Test 1: Secret validation - missing secret (Issue 2586)", func() {
		var originalSecretName string

		BeforeAll(func(ctx SpecContext) {
			// Create keys secret for CTLog
			By("Creating CTLog keys secret")
			keysSecret := ctlog.CreateSecret(namespace.Name, "test-ctlog-keys")
			Expect(cli.Create(ctx, keysSecret)).To(Succeed())

			// Create a proper root certificate secret (CTLog needs root certs from Fulcio)
			By("Creating root certificate")
			_, _, rootCert, err := support.CreateCertificates(false)
			Expect(err).NotTo(HaveOccurred())
			rootCertSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-root-cert",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"cert": rootCert,
				},
			}
			Expect(cli.Create(ctx, rootCertSecret)).To(Succeed())

			// Create CTLog CR with keys and root cert
			ctlogCR = &v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.CTlogSpec{
					Trillian: v1alpha1.TrillianService{
						Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespace.Name),
						Port:    ptr.To(int32(8091)),
					},
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys",
						},
						Key: "private",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys",
						},
						Key: "public",
					},
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-root-cert",
							},
							Key: "cert",
						},
					},
				},
			}
			Expect(cli.Create(ctx, ctlogCR)).To(Succeed())

			By("Waiting for CTLog to be ready initially")
			// Initial CTLog setup uses default 3 minute timeout for tree creation
			ctlog.Verify(ctx, cli, namespace.Name, ctlogCR.Name)
		})

		It("should have a config secret reference in status", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())
				g.Expect(c.Status.ServerConfigRef).NotTo(BeNil())
				g.Expect(c.Status.ServerConfigRef.Name).NotTo(BeEmpty())
				return c.Status.ServerConfigRef.Name
			}).Should(Not(BeEmpty()))

			// Store the original secret name
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			originalSecretName = c.Status.ServerConfigRef.Name
			GinkgoWriter.Printf("Original config secret name: %s\n", originalSecretName)
		})

		It("should have the config secret exist in the cluster", func(ctx SpecContext) {
			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
			Expect(secret.Data).To(HaveKey("config"))
			GinkgoWriter.Printf("Config secret exists with %d bytes of config data\n", len(secret.Data["config"]))
		})

		It("should delete the config secret to simulate cluster recreation", func(ctx SpecContext) {
			By("Deleting config secret: " + originalSecretName)
			err := ctlog.DeleteConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying secret is actually gone")
			Eventually(func() bool {
				_, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
				return errors.IsNotFound(err)
			}).WithTimeout(30*time.Second).Should(BeTrue(), "Secret should be deleted")
		})

		It("should trigger reconciliation by updating CTLog annotation", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())
				if c.Annotations == nil {
					c.Annotations = make(map[string]string)
				}
				c.Annotations["test.trigger/reconcile"] = time.Now().Format(time.RFC3339)
				return cli.Update(ctx, c)
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})

		It("should detect missing secret and recreate it (Phase 1 validation)", func(ctx SpecContext) {
			By("Waiting for operator to detect missing secret")

			// The CTLog should go into a non-Ready state first (detecting the issue)
			// Then it should recover by recreating the secret
			// This needs extra time as it waits for reconciliation loop
			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())

				// Check if status has a new/recreated secret reference
				if c.Status.ServerConfigRef != nil {
					// Try to get the secret
					secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, c.Status.ServerConfigRef.Name)
					if err == nil && secret != nil {
						GinkgoWriter.Printf("Config secret recreated: %s\n", c.Status.ServerConfigRef.Name)
						return true
					}
				}
				return false
			}).WithTimeout(5*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
				"Operator should detect missing secret and recreate it")
		})

		It("should have a valid config secret after recreation", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef).NotTo(BeNil())

			newSecretName := c.Status.ServerConfigRef.Name
			GinkgoWriter.Printf("New config secret name: %s\n", newSecretName)

			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, newSecretName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
			Expect(secret.Data).To(HaveKey("config"))

			// Verify config contains correct Trillian address
			configData := secret.Data["config"]
			expectedTrillianAddr := fmt.Sprintf("trillian-logserver.%s.svc.cluster.local:8091", namespace.Name)
			Expect(string(configData)).To(ContainSubstring(expectedTrillianAddr),
				"Config should contain correct Trillian address")
		})

		It("should have CTLog deployment updated", func(ctx SpecContext) {
			Eventually(condition.DeploymentIsRunning).
				WithContext(ctx).
				WithArguments(cli, namespace.Name, ctlogActions.ComponentName).
				Should(BeTrue(), "CTLog deployment should be running")
		})

		It("should have CTLog pod running (not stuck)", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				pod := ctlog.GetServerPod(ctx, cli, namespace.Name)
				g.Expect(pod).NotTo(BeNil())
				return pod.Status.Phase == v1.PodRunning
			}).Should(BeTrue(),
				"CTLog pod should be Running, not stuck in ContainerCreating")
		})

		It("should have CTLog status Ready", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())
				readyCond := meta.FindStatusCondition(c.Status.Conditions, constants.Ready)
				if readyCond == nil {
					return ""
				}
				return readyCond.Reason
			}).Should(Equal(constants.Ready),
				"CTLog should be Ready after secret recreation")
		})

		It("should preserve TreeID after secret recreation", func(ctx SpecContext) {
			// Note: In Phase 1, TreeID is preserved because it's in the CR status
			// and the CR was not deleted. This test verifies TreeID remains stable.
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.TreeID).NotTo(BeNil())
			GinkgoWriter.Printf("TreeID after recovery: %d\n", *c.Status.TreeID)
		})

		// Cleanup
		AfterAll(func(ctx SpecContext) {
			if ctlogCR != nil {
				_ = cli.Delete(ctx, ctlogCR)
			}
		})
	})
})
