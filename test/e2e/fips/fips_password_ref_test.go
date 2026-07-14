//go:build fips

package fips

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	fulciohelpers "github.com/securesign/operator/test/e2e/support/tas/fulcio"
	rekorhelpers "github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	tsahelpers "github.com/securesign/operator/test/e2e/support/tas/tsa"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FIPS password-ref rejection", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *rhtasv1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-compliant-password")).To(Succeed())
		postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)

		s = securesign.Create(namespace.Name, "test-fips-reject",
			securesign.WithFipsDefaults(namespace.Name),
			securesign.WithProvidedEncryptedCerts(),
		)
		Expect(cli.Create(ctx, s)).To(Succeed())
	})

	findCondition := func(ctx context.Context, conditionType string, getCR func(context.Context) []metav1.Condition) *metav1.Condition {
		conditions := getCR(ctx)
		if conditions == nil {
			return nil
		}
		return meta.FindStatusCondition(conditions, conditionType)
	}

	assertRejectsPasswordRef := func(conditionType string, getCR func(context.Context) []metav1.Condition) {
		It("rejects password-ref with FIPS error", func(ctx SpecContext) {
			Eventually(func(ctx context.Context) *metav1.Condition {
				return findCondition(ctx, conditionType, getCR)
			}).WithContext(ctx).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(
				And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
					WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("FIPS")),
				),
			)
		})

		It("does not retry (terminal error)", func(ctx SpecContext) {
			Consistently(func(ctx context.Context) *metav1.Condition {
				return findCondition(ctx, conditionType, getCR)
			}).WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(
				And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
					WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("FIPS")),
				),
			)
		})
	}

	Describe("CTlog", func() {
		assertRejectsPasswordRef(fipsutil.FIPSCondition, func(ctx context.Context) []metav1.Condition {
			cr := ctlog.Get(ctx, cli, namespace.Name, s.Name)
			if cr == nil {
				return nil
			}
			return cr.GetConditions()
		})
	})

	Describe("Fulcio", func() {
		assertRejectsPasswordRef(fipsutil.FIPSCondition, func(ctx context.Context) []metav1.Condition {
			cr := fulciohelpers.Get(ctx, cli, namespace.Name, s.Name)
			if cr == nil {
				return nil
			}
			return cr.GetConditions()
		})
	})

	Describe("Rekor", func() {
		assertRejectsPasswordRef(fipsutil.FIPSCondition, func(ctx context.Context) []metav1.Condition {
			cr := rekorhelpers.Get(ctx, cli, namespace.Name, s.Name)
			if cr == nil {
				return nil
			}
			return cr.GetConditions()
		})
	})

	Describe("TSA", func() {
		assertRejectsPasswordRef(fipsutil.FIPSCondition, func(ctx context.Context) []metav1.Condition {
			cr := tsahelpers.Get(ctx, cli, namespace.Name, s.Name)
			if cr == nil {
				return nil
			}
			return cr.GetConditions()
		})
	})
})
