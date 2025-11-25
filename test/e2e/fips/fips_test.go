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
			verifyFips(ctx, pod, ctlogactions.DeploymentName, namespace.Name, "/usr/local/bin/ct_server")
		})

		It("Verify creatree job is running in FIPS mode", func(ctx SpecContext) {
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-createtree",
					Namespace: namespace.Name,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "test-createtree",
							Image: images.Registry.Get(images.TrillianCreateTree),
							Args: []string{
								"--admin_server=0.0.0.0:1",
								"--rpc_deadline=10m",
							},

							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			}

			Expect(cli.Create(ctx, p)).To(Succeed())
			DeferCleanup(func() { _ = cli.Delete(ctx, p) })
			Eventually(func(g Gomega) {
				got := &corev1.Pod{}
				g.Expect(cli.Get(ctx, ctrlclient.ObjectKeyFromObject(p), got)).To(Succeed())
				g.Expect(got.Status.Phase).ToNot(Equal(corev1.PodPending))
			}).WithContext(ctx).Should(Succeed())
			verifyFips(ctx, p, "test-createtree", namespace.Name, "/createtree")
		})

		It("Verify fulcio is running in FIPS mode", func(ctx SpecContext) {
			server := fulciohelpers.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).ToNot(BeNil())
			verifyFips(ctx, server, fulcioactions.DeploymentName, namespace.Name, "/usr/local/bin/fulcio-server")
		})

		It("Verify rekor-server is running in FIPS mode", func(ctx SpecContext) {
			server := rekorhelpers.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).ToNot(BeNil())
			verifyFips(ctx, server, rekoractions.ServerDeploymentName, namespace.Name, "/usr/local/bin/rekor-server")
		})

		It("Verify backfill-redis job is running in FIPS mode", func(ctx SpecContext) {
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-backfill-redis",
					Namespace: namespace.Name,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "backfill-redis",
							Image: images.Registry.Get(images.BackfillRedis),
							Args: []string{
								"--rekor-address=https://0.0.0.0:1",
								"--rekor-retry-count=20",
								"--redis-hostname=0.0.0.0",
								"--redis-port=1",
								"--start=0",
								"--end=0",
								" --concurrency=1",
							},

							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			}
			Expect(cli.Create(ctx, p)).To(Succeed())
			DeferCleanup(func() { _ = cli.Delete(ctx, p) })
			Eventually(func(g Gomega) {
				got := &corev1.Pod{}
				g.Expect(cli.Get(ctx, ctrlclient.ObjectKeyFromObject(p), got)).To(Succeed())
				g.Expect(got.Status.Phase).ToNot(Equal(corev1.PodPending))
			}).WithContext(ctx).Should(Succeed())
			verifyFips(ctx, p, "backfill-redis", namespace.Name, "/usr/local/bin/backfill-redis")
		})

		It("Verify rekor-redis is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.RedisDeploymentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			redis := &list.Items[0]
			verifyFips(ctx, redis, rekoractions.RedisDeploymentName, namespace.Name, "/usr/bin/redis-server")
		})

		It("Verify rekor-ui is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.UIComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			ui := &list.Items[0]

			host, err := k8ssupport.ExecInPodWithOutput(ctx, ui.Name, rekoractions.SearchUiDeploymentName, namespace.Name,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(host))).To(Equal("1"))

			node, err := k8ssupport.ExecInPodWithOutput(ctx, ui.Name, rekoractions.SearchUiDeploymentName, namespace.Name,
				"node", "-p", "require('crypto').getFips()",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(node))).To(Equal("1"))
		})

		It("Verify TUF Server is running in FIPS mode", func(ctx SpecContext) {
			server := tufhelpers.GetServerPod(ctx, cli, namespace.Name)
			Expect(server).ToNot(BeNil())
			verifyFips(ctx, server, tufconstants.ContainerName, namespace.Name, "/usr/sbin/httpd")
		})

		It("Verify tuf init job is running in FIPS mode", func(ctx SpecContext) {
			p := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fips-tuf-init",
					Namespace: namespace.Name,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "fips-tuf-init",
							Image:   images.Registry.Get(images.Tuf),
							Command: []string{"sh", "-c", "sleep 300"},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								RunAsNonRoot:             ptr.To(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			}

			Expect(cli.Create(ctx, p)).To(Succeed())
			DeferCleanup(func() { _ = cli.Delete(ctx, p) })

			Eventually(func(g Gomega) corev1.PodPhase {
				got := &corev1.Pod{}
				g.Expect(cli.Get(ctx, ctrlclient.ObjectKeyFromObject(p), got)).To(Succeed())
				return got.Status.Phase
			}).WithContext(ctx).Should(Equal(corev1.PodRunning))

			out, err := k8ssupport.ExecInPodWithOutput(ctx, p.Name, "fips-tuf-init", p.Namespace,
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(out))).To(Equal("1"))

			prov, err := k8ssupport.ExecInPodWithOutput(ctx, p.Name, "fips-tuf-init", p.Namespace,
				"openssl", "list", "-providers", "-verbose",
			)
			Expect(err).ToNot(HaveOccurred())
			s := string(prov)
			Expect(s).To(ContainSubstring("\n  fips\n"))
			Expect(s).To(ContainSubstring("\n    status: active\n"))

			lddOut, err := k8ssupport.ExecInPodWithOutput(ctx, p.Name, "fips-tuf-init", p.Namespace,
				"ldd", "/usr/bin/tuftool",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(lddOut)).To(ContainSubstring("libcrypto.so.3"))
			Expect(string(lddOut)).To(ContainSubstring("libssl.so.3"))

		})

		It("Verify TSA is running in FIPS mode", func(ctx SpecContext) {
			server := tsahelpers.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).ToNot(BeNil())
			verifyFips(ctx, server, tsaactions.DeploymentName, namespace.Name, "/usr/local/bin/timestamp-server")
		})

		It("Verify trillian logserver is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogServerComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			server := &list.Items[0]
			verifyFips(ctx, server, trillianactions.LogServerComponentName, namespace.Name, "/trillian_log_server")
		})

		It("Verify trillian logsigner is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogSignerComponentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			signer := &list.Items[0]
			verifyFips(ctx, signer, trillianactions.LogSignerComponentName, namespace.Name, "/trillian_log_signer")
		})

		It("Verify trillian db is running in FIPS mode", func(ctx SpecContext) {
			list := &v1.PodList{}
			Expect(cli.List(ctx, list,
				ctrlclient.InNamespace(namespace.Name),
				ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.DbDeploymentName},
			)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
			db := &list.Items[0]
			verifyFips(ctx, db, trillianactions.DbDeploymentName, namespace.Name, "/usr/libexec/mariadbd")
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
			verifyFips(ctx, mgr, mgr.Spec.Containers[0].Name, mgr.Namespace, "/manager")
		})
	})
})

func verifyFips(ctx SpecContext, pod *v1.Pod, containerName, namespace string, expectedExe string) {
	out, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"cat", "/proc/sys/crypto/fips_enabled", //FIPS is enabled at the kernel level
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(out))).To(Equal("1"))

	if expectedExe != "" {
		exe, err := k8ssupport.ExecInPodWithOutput(ctx,
			pod.Name, containerName, namespace,
			"readlink", "-f", "/proc/1/exe", //confirm process 1 is expected binary
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(string(exe))).To(Equal(expectedExe))
	}

	libcryptoMap, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"sh", "-c", `grep -E 'libcrypto\.so(\.3)?' /proc/1/maps | head -n 1`, //confirm process 1 has loaded openssl
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(libcryptoMap))).To(ContainSubstring("libcrypto.so"))

	fips, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"sh", "-c", `grep -F 'ossl-modules/fips.so' /proc/1/maps | head -n 1`, //confirm process 1 has loaded the openssl fips module
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(fips))).To(ContainSubstring("ossl-modules/fips.so"))
}
