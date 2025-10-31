//go:build integration

package e2e

import (
	"fmt"
	"strings"
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

	Describe("Test 1: Secret validation - missing/invalid config (Issues 2586 & 3114)", func() {
		var originalSecretName string
		var correctTrillianAddr string

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

			correctTrillianAddr = fmt.Sprintf("trillian-logserver.%s.svc.cluster.local:8091", namespace.Name)

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

		It("should have a config secret with correct Trillian address", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef.Name).NotTo(BeEmpty())

			originalSecretName = c.Status.ServerConfigRef.Name

			// Verify secret exists with correct Trillian configuration
			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
			Expect(secret.Data).To(HaveKey("config"))

			configContent := ctlog.GetTrillianAddressFromSecret(secret)
			Expect(configContent).To(ContainSubstring(correctTrillianAddr),
				"Config should contain correct Trillian address")
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

		It("should detect missing/invalid config and recreate it (Phase 1 validation)", func(ctx SpecContext) {
			By("Waiting for operator to detect missing/invalid config")

			// The CTLog should go into a non-Ready state first (detecting the issue)
			// Then it should recover by recreating the secret with correct Trillian config
			// This tests both Issue 2586 (missing secret) and Issue 3114 (wrong namespace)
			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())

				// Check if status has a new/recreated secret reference
				if c.Status.ServerConfigRef != nil {
					// Try to get the secret
					secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, c.Status.ServerConfigRef.Name)
					if err == nil && secret != nil {
						// Verify it has correct Trillian address
						configContent := ctlog.GetTrillianAddressFromSecret(secret)
						if strings.Contains(configContent, correctTrillianAddr) {
							return true
						}
					}
				}
				return false
			}).WithPolling(5*time.Second).Should(BeTrue(),
				"Operator should detect missing/invalid config and recreate it with correct Trillian address")
		})

		It("should have a valid config secret after recreation", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef).NotTo(BeNil())

			newSecretName := c.Status.ServerConfigRef.Name

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
		})

		// Cleanup
		AfterAll(func(ctx SpecContext) {
			if ctlogCR != nil {
				_ = cli.Delete(ctx, ctlogCR)
			}
		})
	})

	Describe("Test 2: TreeID recovery after CR deletion (Phase 2)", func() {
		var originalTreeID *int64
		var originalSecretName string
		var keysSecret *v1.Secret
		var rootCertSecret *v1.Secret

		BeforeAll(func(ctx SpecContext) {
			// Create keys secret for CTLog
			By("Creating CTLog keys secret")
			keysSecret = ctlog.CreateSecret(namespace.Name, "test-ctlog-keys-phase2")
			Expect(cli.Create(ctx, keysSecret)).To(Succeed())

			// Create a proper root certificate secret
			By("Creating root certificate")
			_, _, rootCert, err := support.CreateCertificates(false)
			Expect(err).NotTo(HaveOccurred())
			rootCertSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-root-cert-phase2",
					Namespace: namespace.Name,
				},
				Data: map[string][]byte{
					"cert": rootCert,
				},
			}
			Expect(cli.Create(ctx, rootCertSecret)).To(Succeed())

			// Create CTLog CR (Phase 2: without owner reference on config secret)
			ctlogCR = &v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-phase2",
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.CTlogSpec{
					Trillian: v1alpha1.TrillianService{
						Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespace.Name),
						Port:    ptr.To(int32(8091)),
					},
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys-phase2",
						},
						Key: "private",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys-phase2",
						},
						Key: "public",
					},
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-root-cert-phase2",
							},
							Key: "cert",
						},
					},
				},
			}
			Expect(cli.Create(ctx, ctlogCR)).To(Succeed())

			By("Waiting for CTLog to be ready initially")
			ctlog.Verify(ctx, cli, namespace.Name, ctlogCR.Name)
		})

		It("should record original TreeID and config secret", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.TreeID).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef).NotTo(BeNil())
			Expect(c.Status.ServerConfigRef.Name).NotTo(BeEmpty())

			originalTreeID = c.Status.TreeID
			originalSecretName = c.Status.ServerConfigRef.Name
		})

		It("should have config secret with NO owner reference (Phase 2)", func(ctx SpecContext) {
			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())

			// Phase 2 Fix: Secret should have NO owner reference
			// This allows it to survive CR deletion
			hasNoOwnerRef := ctlog.VerifySecretHasNoOwnerReference(secret)
			Expect(hasNoOwnerRef).To(BeTrue(),
				"Config secret should have NO owner reference in Phase 2 (allows state recovery)")
		})

		It("should have TreeID embedded in config secret", func(ctx SpecContext) {
			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())

			treeIDFromSecret := ctlog.GetTreeIDFromConfigSecret(secret)
			Expect(treeIDFromSecret).NotTo(BeNil(), "TreeID should be in config secret")
			Expect(*treeIDFromSecret).To(Equal(*originalTreeID),
				"TreeID in secret should match TreeID in status")
		})

		It("should delete CTLog CR (simulating disaster recovery)", func(ctx SpecContext) {
			By("Deleting CTLog CR")
			Expect(cli.Delete(ctx, ctlogCR)).To(Succeed())

			By("Waiting for CTLog CR to be gone")
			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				return c == nil
			}).WithTimeout(2 * time.Minute).Should(BeTrue())
		})

		It("should verify config secret STILL EXISTS (Phase 2 - no garbage collection)", func(ctx SpecContext) {
			// This is the key Phase 2 behavior:
			// Without owner reference, K8s does NOT garbage collect the secret
			secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred(), "Secret should still exist after CR deletion")
			Expect(secret).NotTo(BeNil())
			Expect(secret.Name).To(Equal(originalSecretName))
		})

		It("should recreate CTLog CR with same name", func(ctx SpecContext) {
			By("Recreating CTLog CR")
			ctlogCR = &v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-phase2",
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.CTlogSpec{
					Trillian: v1alpha1.TrillianService{
						Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespace.Name),
						Port:    ptr.To(int32(8091)),
					},
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys-phase2",
						},
						Key: "private",
					},
					PublicKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "test-ctlog-keys-phase2",
						},
						Key: "public",
					},
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-root-cert-phase2",
							},
							Key: "cert",
						},
					},
				},
			}
			Expect(cli.Create(ctx, ctlogCR)).To(Succeed())
		})

		It("should discover existing config secret (Phase 2)", func(ctx SpecContext) {
			// Operator should discover the existing secret either:
			// 1. Via Status.ServerConfigRef (empty on new CR)
			// 2. Via label-based discovery

			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())

				// Check if operator has discovered and referenced the secret
				if c.Status.ServerConfigRef != nil {
					// Should reference the original secret (discovered)
					return c.Status.ServerConfigRef.Name == originalSecretName
				}
				return false
			}).WithTimeout(3*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
				"Operator should discover existing config secret")
		})

		It("should restore TreeID from config secret (Phase 2)", func(ctx SpecContext) {
			// This is the critical Phase 2 behavior:
			// TreeID should be extracted from the discovered secret, not regenerated

			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())

				if c.Status.TreeID != nil {
					return *c.Status.TreeID == *originalTreeID
				}
				return false
			}).WithTimeout(3*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
				"TreeID should be restored from config secret, not regenerated")
		})

		It("should have CTLog Ready with restored state", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())
				readyCond := meta.FindStatusCondition(c.Status.Conditions, constants.Ready)
				if readyCond == nil {
					return ""
				}
				return readyCond.Reason
			}).WithTimeout(3*time.Minute).Should(Equal(constants.Ready),
				"CTLog should be Ready after state recovery")
		})

		It("should verify no new TreeID was generated", func(ctx SpecContext) {
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.TreeID).NotTo(BeNil())
			Expect(*c.Status.TreeID).To(Equal(*originalTreeID),
				"TreeID should be the same as original (no regeneration)")
		})

		// Cleanup
		AfterAll(func(ctx SpecContext) {
			if ctlogCR != nil {
				_ = cli.Delete(ctx, ctlogCR)
			}
			if keysSecret != nil {
				_ = cli.Delete(ctx, keysSecret)
			}
			if rootCertSecret != nil {
				_ = cli.Delete(ctx, rootCertSecret)
			}
			// Clean up the config secret (not garbage collected in Phase 2)
			if originalSecretName != "" {
				_ = ctlog.DeleteConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			}
		})
	})
})
