//go:build custom_install

package custom_install

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogAction "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioAction "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekorAction "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianAction "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaAction "github.com/securesign/operator/internal/controller/tsa/actions"
	tufAction "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const dummyTag = ":dummy@sha256:"

func withRelatedImages() optManagerPod {
	return func(pod *v1.Pod) {
		for _, img := range images.Images {
			value := images.Registry.Get(img)
			value = strings.ReplaceAll(value, "@sha256:", dummyTag)

			pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, v1.EnvVar{
				Name:  string(img),
				Value: value,
			})
		}
	}
}

func withReplicas(replicas int32) securesign.Opts {
	return func(s *v1alpha1.Securesign) {
		s.Spec.Rekor.Replicas = ptr.To(replicas)
		s.Spec.Rekor.RekorSearchUI.Replicas = ptr.To(replicas)
		s.Spec.Fulcio.Replicas = ptr.To(replicas)
		s.Spec.Ctlog.Replicas = ptr.To(replicas)
		s.Spec.Tuf.Replicas = ptr.To(replicas)
		s.Spec.Trillian.LogServer.Replicas = ptr.To(replicas)
		s.Spec.Trillian.LogSigner.Replicas = ptr.To(replicas)
		s.Spec.TimestampAuthority.Replicas = ptr.To(replicas)
	}
}

var _ = Describe("rolling upgrade with replicas", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	Describe("Successful installation of manager", func() {
		BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
			namespace = new
		}))

		AfterAll(func(ctx SpecContext) {
			_ = cli.Delete(ctx, s)
			// wait until object has been deleted. Manager need to handle finalizer
			Eventually(func(ctx context.Context) error {
				return cli.Get(ctx, client.ObjectKeyFromObject(s), &v1alpha1.Securesign{})
			}).WithContext(ctx).Should(And(HaveOccurred(), WithTransform(apierrors.IsNotFound, BeTrue())))
			uninstallOperator(ctx, cli, namespace.Name)
		})

		BeforeAll(func(ctx SpecContext) {
			installOperator(ctx, cli, namespace.Name)
		})

		It("Install securesign", func(ctx SpecContext) {
			s = securesign.Create(namespace.Name, "test",
				securesign.WithDefaults(),
				securesign.WithNFSPVC(),
				withReplicas(2),
				func(v *v1alpha1.Securesign) {
					v.Spec.Fulcio.Config = v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  "sigstore",
								IssuerURL: "https://oauth2.sigstore.dev/auth",
								Issuer:    "https://oauth2.sigstore.dev/auth",
								Type:      "email",
							},
						},
					}
				},
			)
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Change related images", func(ctx SpecContext) {
			uninstallOperator(ctx, cli, namespace.Name)

			By("install operator with modified related images")
			installOperator(ctx, cli, s.Namespace, withRelatedImages())
		})

		It("Verify rolling update done", func(ctx SpecContext) {
			By("verify that deployment use new images")
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, ctlogAction.DeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, fulcioAction.DeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, tufAction.DeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, tsaAction.DeploymentName)

			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, rekorAction.RedisDeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, rekorAction.ServerDeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, rekorAction.SearchUiDeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, rekorAction.SearchUiDeploymentName)

			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, trillianAction.LogserverDeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, trillianAction.LogsignerDeploymentName)
			verifyDeploymentHasNewImage(ctx, cli, s.Namespace, trillianAction.DbDeploymentName)

			By("verify that all components are running")
			tas.VerifyAllComponents(ctx, cli, s, true)
		})
	})
})

func verifyDeploymentHasNewImage(ctx context.Context, cli client.Client, namespace, name string) {
	Eventually(func(g Gomega, ctx context.Context) {
		dep := &appsv1.Deployment{}
		g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dep)).To(Succeed())

		for _, container := range dep.Spec.Template.Spec.Containers {
			g.Expect(container.Image).To(ContainSubstring(dummyTag))
		}
	}).WithContext(ctx).Should(Succeed())
}
