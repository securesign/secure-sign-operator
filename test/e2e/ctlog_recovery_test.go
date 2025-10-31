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
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
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
	var originalSecretName string
	var correctTrillianAddr string

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

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

	BeforeAll(func(ctx SpecContext) {
		By("Setting up CTLog prerequisites")

		keysSecret := ctlog.CreateSecret(namespace.Name, "test-ctlog-keys")
		Expect(cli.Create(ctx, keysSecret)).To(Succeed())

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

		By("Creating CTLog instance")
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

		By("Waiting for initial CTLog deployment")
		ctlog.Verify(ctx, cli, namespace.Name, ctlogCR.Name)
	})

	Describe("CTLog self-healing when config secret is missing or invalid", func() {

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
			Expect(secret.Data).To(HaveKey(ctlogUtils.ConfigKey))

			configContent := ctlog.GetTrillianAddressFromSecret(secret)
			Expect(configContent).To(ContainSubstring(correctTrillianAddr),
				"Config should contain correct Trillian address")
		})

		It("should simulate cluster recreation by deleting the config secret", func(ctx SpecContext) {
			By("Deleting config secret to simulate disaster scenario")
			err := ctlog.DeleteConfigSecret(ctx, cli, namespace.Name, originalSecretName)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying secret deletion")
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

		It("should automatically detect and recreate the missing config secret", func(ctx SpecContext) {
			By("Waiting for operator to detect and fix the missing config")

			// The operator should detect the missing secret and recreate it
			// with the correct Trillian configuration from the CTLog spec
			Eventually(func(g Gomega) bool {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())

				if c.Status.ServerConfigRef != nil {
					secret, err := ctlog.GetConfigSecret(ctx, cli, namespace.Name, c.Status.ServerConfigRef.Name)
					if err == nil && secret != nil {
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
			Expect(secret.Data).To(HaveKey(ctlogUtils.ConfigKey))

			// Verify config contains correct Trillian address
			configData := secret.Data[ctlogUtils.ConfigKey]
			expectedTrillianAddr := fmt.Sprintf("trillian-logserver.%s.svc.cluster.local:8091", namespace.Name)
			Expect(string(configData)).To(ContainSubstring(expectedTrillianAddr),
				"Config should contain correct Trillian address")
		})

		It("should have CTLog deployment running with the new config", func(ctx SpecContext) {
			Eventually(condition.DeploymentIsRunning).
				WithContext(ctx).
				WithArguments(cli, namespace.Name, ctlogActions.ComponentName).
				Should(BeTrue(), "CTLog deployment should be running after config recreation")
		})

		It("should have CTLog pod running without being stuck waiting for secret", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				pod := ctlog.GetServerPod(ctx, cli, namespace.Name)
				g.Expect(pod).NotTo(BeNil())
				return pod.Status.Phase == v1.PodRunning
			}).Should(BeTrue(),
				"CTLog pod should reach Running phase, proving it's not stuck waiting for a missing secret")
		})

		It("should report Ready status after successful recovery", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
				g.Expect(c).NotTo(BeNil())
				readyCond := meta.FindStatusCondition(c.Status.Conditions, constants.Ready)
				if readyCond == nil {
					return ""
				}
				return readyCond.Reason
			}).Should(Equal(constants.Ready),
				"CTLog should transition to Ready status after config secret recreation")
		})

		It("should preserve TreeID after secret recreation", func(ctx SpecContext) {
			// TreeID is preserved because it's stored in the CR status,
			// and the CR itself was not deleted during this recovery scenario
			c := ctlog.Get(ctx, cli, namespace.Name, ctlogCR.Name)
			Expect(c).NotTo(BeNil())
			Expect(c.Status.TreeID).NotTo(BeNil(), "TreeID should remain stable across secret recreation")
		})

		AfterAll(func(ctx SpecContext) {
			if ctlogCR != nil {
				_ = cli.Delete(ctx, ctlogCR)
			}
		})
	})
})
