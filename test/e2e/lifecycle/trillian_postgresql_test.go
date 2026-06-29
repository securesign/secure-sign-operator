//go:build integration

package lifecycle

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Trillian install with byodb", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var t *rhtasv1.Trillian

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {

		t = &rhtasv1.Trillian{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "postgresql-test",
			},
			Spec: rhtasv1.TrillianSpec{
				Auth: &rhtasv1.Auth{
					Env: postgresql.AuthEnvVars(namespace.Name, postgresql.DefaultSecretName),
				},
				Db: rhtasv1.TrillianDB{
					Create:   ptr.To(false),
					Provider: postgresql.Provider,
					Uri:      postgresql.ConnectionURI,
				},
			},
		}
	})

	Describe("Install with byodb", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)

			Expect(cli.Create(ctx, t)).To(Succeed())
		})

		It("Trillian is running", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, t.Namespace, t.Name, false)

			podList := &v1.PodList{}
			Expect(cli.List(ctx, podList, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{
				"app.kubernetes.io/part-of": "trusted-artifact-signer",
			})).To(Succeed())
			Expect(podList.Items).To(HaveLen(2))

			for _, pod := range podList.Items {
				log, err := testSupportKubernetes.GetPodLogs(ctx, pod.Name, pod.Labels["app.kubernetes.io/component"], pod.Namespace)
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.ToLower(log)).ToNot(ContainSubstring("error"))
				for _, c := range pod.Status.ContainerStatuses {
					Expect(c.RestartCount).To(BeNumerically("==", 0))
				}
			}
		})
	})
})
