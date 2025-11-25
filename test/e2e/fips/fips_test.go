//go:build fips

package fips

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianactions "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaactions "github.com/securesign/operator/internal/controller/tsa/actions"
	tufconstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	k8ssupport "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	fulciohelpers "github.com/securesign/operator/test/e2e/support/tas/fulcio"
	rekorhelpers "github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	tsahelpers "github.com/securesign/operator/test/e2e/support/tas/tsa"
	tufhelpers "github.com/securesign/operator/test/e2e/support/tas/tuf"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Securesign FIPS", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithSearchUI(),
			securesign.WithMonitoring(),
			func(v *v1alpha1.Securesign) {
				// cover SECURESIGN-2694
				v.Spec.Rekor.Attestations.Enabled = ptr.To(false)
			},
			func(v *v1alpha1.Securesign) {
				v.Spec.Tuf.ExternalAccess.RouteSelectorLabels = map[string]string{"foo": "bar"}
			},
		)
	})

	Describe("Install into FIPS Enabled cluster", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Verify ctlog is running in FIPS mode", func(ctx SpecContext) {
			pod := ctlog.GetServerPod(ctx, cli, namespace.Name)
			Expect(pod).ToNot(BeNil())
			out, err := k8ssupport.ExecInPodWithOutput(ctx,
				pod.Name, ctlogactions.DeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify creatree job is running in FIPS mode", func(ctx SpecContext) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-createtree",
					Namespace: namespace.Name,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "test-createtree",
									Image: images.Registry.Get(images.TrillianCreateTree),
									Command: []string{
										"sh", "-c", "sleep 300",
									},
								},
							},
						},
					},
				},
			}

			Expect(cli.Create(ctx, job)).To(Succeed())
			DeferCleanup(func() {
				_ = cli.Delete(ctx, job)
			})

			list := &v1.PodList{}
			Eventually(func(g Gomega) {
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(namespace.Name),
					ctrlclient.MatchingLabels{"batch.kubernetes.io/job-name": "fips-createtree"},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())
			testPod := &list.Items[0]

			Eventually(func(g Gomega) string {
				out, err := k8ssupport.ExecInPodWithOutput(ctx, testPod.Name, "test-createtree", testPod.Namespace,
					"cat", "/proc/sys/crypto/fips_enabled",
				)
				g.Expect(err).ToNot(HaveOccurred())
				return strings.TrimSpace(string(out))
			}).WithContext(ctx).Should(Equal("1"))
		})

		It("Verify fulcio is running in FIPS mode", func(ctx SpecContext) {
			server := fulciohelpers.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).ToNot(BeNil())

			out, err := k8ssupport.ExecInPodWithOutput(ctx, server.Name, fulcioactions.DeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify rekor-server is running in FIPS mode", func(ctx SpecContext) {
			server := rekorhelpers.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).ToNot(BeNil())

			out, err := k8ssupport.ExecInPodWithOutput(ctx, server.Name, rekoractions.ServerDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify backfill-redis job is running in FIPS mode", func(ctx SpecContext) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-backfill-redis",
					Namespace: namespace.Name,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "backfill-redis",
									Image: images.Registry.Get(images.BackfillRedis),
									Command: []string{
										"sh", "-c", "sleep 300",
									},
								},
							},
						},
					},
				},
			}

			Expect(cli.Create(ctx, job)).To(Succeed())
			DeferCleanup(func() {
				_ = cli.Delete(ctx, job)
			})

			list := &v1.PodList{}
			Eventually(func(g Gomega) {
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(namespace.Name),
					ctrlclient.MatchingLabels{"batch.kubernetes.io/job-name": "fips-backfill-redis"},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())
			testPod := &list.Items[0]

			Eventually(func(g Gomega) string {
				out, err := k8ssupport.ExecInPodWithOutput(ctx, testPod.Name, "backfill-redis", testPod.Namespace,
					"cat", "/proc/sys/crypto/fips_enabled",
				)
				g.Expect(err).ToNot(HaveOccurred())
				return strings.TrimSpace(string(out))
			}).WithContext(ctx).Should(Equal("1"))
		})

		It("Verify rekor-redis is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.RedisDeploymentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			redis := &list.Items[0]

			out, err := k8ssupport.ExecInPodWithOutput(ctx, redis.Name, rekoractions.RedisDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify rekor-ui is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.UIComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			ui := &list.Items[0]

			out, err := k8ssupport.ExecInPodWithOutput(ctx, ui.Name, rekoractions.SearchUiDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify TUF is running in FIPS mode", func(ctx SpecContext) {
			server := tufhelpers.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).ToNot(BeNil())

			out, err := k8ssupport.ExecInPodWithOutput(ctx, server.Name, tufconstants.ContainerName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify tuf init job is running in FIPS mode", func(ctx SpecContext) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-tuf-init",
					Namespace: namespace.Name,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "test-tuf-init",
									Image: images.Registry.Get(images.Tuf),
									Command: []string{
										"sh", "-c", "sleep 300",
									},
								},
							},
						},
					},
				},
			}

			Expect(cli.Create(ctx, job)).To(Succeed())
			DeferCleanup(func() {
				_ = cli.Delete(ctx, job)
			})

			list := &v1.PodList{}
			Eventually(func(g Gomega) {
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(namespace.Name),
					ctrlclient.MatchingLabels{"batch.kubernetes.io/job-name": "fips-tuf-init"},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())
			testPod := &list.Items[0]

			Eventually(func(g Gomega) string {
				out, err := k8ssupport.ExecInPodWithOutput(ctx, testPod.Name, "test-tuf-init", testPod.Namespace,
					"cat", "/proc/sys/crypto/fips_enabled",
				)
				g.Expect(err).ToNot(HaveOccurred())
				return strings.TrimSpace(string(out))
			}).WithContext(ctx).Should(Equal("1"))
		})

		It("Verify TSA is running in FIPS mode", func(ctx SpecContext) {
			server := tsahelpers.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).ToNot(BeNil())

			out, err := k8ssupport.ExecInPodWithOutput(ctx, server.Name, tsaactions.DeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify trillian logserver is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogServerComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			server := &list.Items[0]

			out, err := k8ssupport.ExecInPodWithOutput(ctx, server.Name, trillianactions.LogserverDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify trillian logsigner is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogSignerComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			signer := &list.Items[0]

			out, err := k8ssupport.ExecInPodWithOutput(ctx, signer.Name, trillianactions.LogsignerDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify trillian db is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.DbDeploymentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			db := &list.Items[0]

			out, err := k8ssupport.ExecInPodWithOutput(ctx, db.Name, trillianactions.DbComponentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

		It("Verify Controller manager is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.MatchingLabels{"control-plane": "operator-controller-manager"},
			)).To(Succeed())
			Expect(list.Items).ToNot(BeEmpty())

			var mgr *v1.Pod
			for i := range list.Items {
				pod := &list.Items[i]
				if strings.Contains(pod.Spec.ServiceAccountName, "rhtas-operator") {
					mgr = pod
					break
				}
			}

			Expect(mgr).ToNot(BeNil())
			out, err := k8ssupport.ExecInPodWithOutput(ctx, mgr.Name, "manager", mgr.Namespace,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))
		})

	})
})
