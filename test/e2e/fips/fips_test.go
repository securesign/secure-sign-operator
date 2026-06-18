//go:build fips

package fips

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
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
	olmhelpers "github.com/securesign/operator/test/e2e/support/kubernetes/olm"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	fulciohelpers "github.com/securesign/operator/test/e2e/support/tas/fulcio"
	rekorhelpers "github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	tsahelpers "github.com/securesign/operator/test/e2e/support/tas/tsa"
	tufhelpers "github.com/securesign/operator/test/e2e/support/tas/tuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Securesign FIPS Strict Mode (fips140=only)", Ordered, func() {
	cli, _ := support.CreateClient()

	var namespace *v1.Namespace
	var s *rhtasv1.Securesign

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-compliant-password")).To(Succeed())
		postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)

		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithGeneratedCerts(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
			securesign.WithSearchUI(),
			securesign.WithMonitoring(),
			func(v *rhtasv1.Securesign) {
				v.Spec.Rekor.Attestations.Enabled = ptr.To(false)
				v.Spec.Ctlog.Monitoring.TLog.Enabled = true
				v.Spec.Ctlog.Monitoring.TLog.Interval = metav1.Duration{Duration: 10 * time.Second}
				v.Spec.Rekor.Monitoring.TLog.Enabled = true
				v.Spec.Rekor.Monitoring.TLog.Interval = metav1.Duration{Duration: 10 * time.Second}
			},
			securesign.WithExternalPostgresDB(namespace.Name, postgresql.DefaultSecretName),
		)
	})

	Describe("Install into FIPS cluster", func() {
		BeforeAll(func(ctx SpecContext) {
			// Patch the operator with GODEBUG=fips140=only (via CSV if OLM-managed,
			// otherwise directly) and wait for a ready pod with the env var.
			operatorDep := steps.FindOperatorDeployment(ctx, cli)
			godebug := v1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}
			olmhelpers.PatchCSVDeploymentEnv(ctx, cli, operatorDep.Namespace, operatorDep.Name, operatorDep.Spec.Template.Spec.Containers[0].Name, godebug)

			mgr := steps.WaitForOperatorPodWithEnv(ctx, cli, godebug)
			verifyFipsGoNative(ctx, mgr, mgr.Spec.Containers[0].Name, mgr.Namespace, "/manager")
		})

		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, false, true)
		})

		getNamespace := func() string { return namespace.Name }
		godebug := v1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}

		It("Verify ctlog is running in FIPS mode", func(ctx SpecContext) {
			var pod *v1.Pod
			Eventually(func() *v1.Pod {
				pod = ctlog.GetServerPod(ctx, cli, getNamespace())
				return pod
			}).WithContext(ctx).ShouldNot(BeNil())
			verifyFipsGoNative(ctx, pod, ctlogactions.DeploymentName, getNamespace(), "/usr/local/bin/ct_server")
		})

		It("Verify ctlog-monitor is running in FIPS mode", func(ctx SpecContext) {
			var pod *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: ctlogactions.MonitorComponentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				g.Expect(list.Items[0].Status.Phase).To(Equal(v1.PodRunning))
				pod = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, pod, ctlogactions.MonitorStatefulSetName, getNamespace(), "/ctlog_monitor")
		})

		It("Verify rekor-monitor is running in FIPS mode", func(ctx SpecContext) {
			var pod *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.MonitorComponentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				g.Expect(list.Items[0].Status.Phase).To(Equal(v1.PodRunning))
				pod = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, pod, rekoractions.MonitorStatefulSetName, getNamespace(), "/rekor_monitor")
		})

		It("Verify fulcio is running in FIPS mode", func(ctx SpecContext) {
			var server *v1.Pod
			Eventually(func() *v1.Pod {
				server = fulciohelpers.GetServerPod(ctx, cli, getNamespace())()
				return server
			}).WithContext(ctx).ShouldNot(BeNil())
			verifyFipsGoNative(ctx, server, fulcioactions.DeploymentName, getNamespace(), "/usr/local/bin/fulcio-server")
		})

		It("Verify rekor-server is running in FIPS mode", func(ctx SpecContext) {
			var server *v1.Pod
			Eventually(func() *v1.Pod {
				server = rekorhelpers.GetServerPod(ctx, cli, getNamespace())
				return server
			}).WithContext(ctx).ShouldNot(BeNil())
			verifyFipsGoNative(ctx, server, rekoractions.ServerDeploymentName, getNamespace(), "/usr/local/bin/rekor-server")
		})

		It("Verify rekor-redis is running in FIPS mode", func(ctx SpecContext) {
			var redis *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.RedisDeploymentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				redis = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())
			verifyFipsOpenSSL(ctx, redis, rekoractions.RedisDeploymentName, getNamespace(), "/usr/bin/redis-server")
		})

		It("Verify rekor-ui is running in FIPS mode", func(ctx SpecContext) {
			var ui *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: rekoractions.UIComponentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				ui = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())

			verifyGodebugOnly(ctx, ui, rekoractions.SearchUiDeploymentName, getNamespace())

			host, err := k8ssupport.ExecInPodWithOutput(ctx, ui.Name, rekoractions.SearchUiDeploymentName, getNamespace(),
				"cat", "/proc/sys/crypto/fips_enabled",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(host))).To(Equal("1"))

			node, err := k8ssupport.ExecInPodWithOutput(ctx, ui.Name, rekoractions.SearchUiDeploymentName, getNamespace(),
				"node", "-p", "require('crypto').getFips()",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(string(node))).To(Equal("1"))
		})

		It("Verify TUF Server is running in FIPS mode", func(ctx SpecContext) {
			var server *v1.Pod
			Eventually(func() *v1.Pod {
				server = tufhelpers.GetServerPod(ctx, cli, getNamespace())
				return server
			}).WithContext(ctx).ShouldNot(BeNil())
			verifyFipsOpenSSL(ctx, server, tufconstants.ContainerName, getNamespace(), "/usr/sbin/httpd")
		})

		It("Verify TSA is running in FIPS mode", func(ctx SpecContext) {
			var server *v1.Pod
			Eventually(func() *v1.Pod {
				server = tsahelpers.GetServerPod(ctx, cli, getNamespace())()
				return server
			}).WithContext(ctx).ShouldNot(BeNil())
			verifyFipsGoNative(ctx, server, tsaactions.DeploymentName, getNamespace(), "/usr/local/bin/timestamp-server")
		})

		It("Verify trillian logserver is running in FIPS mode", func(ctx SpecContext) {
			var server *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogServerComponentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				server = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, server, trillianactions.LogServerComponentName, getNamespace(), "/trillian_log_server")
		})

		It("Verify trillian logsigner is running in FIPS mode", func(ctx SpecContext) {
			var signer *v1.Pod
			Eventually(func(g Gomega, ctx context.Context) {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list,
					ctrlclient.InNamespace(getNamespace()),
					ctrlclient.MatchingLabels{labels.LabelAppComponent: trillianactions.LogSignerComponentName},
				)).To(Succeed())
				g.Expect(list.Items).To(HaveLen(1))
				signer = &list.Items[0]
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, signer, trillianactions.LogSignerComponentName, getNamespace(), "/trillian_log_signer")
		})

		It("Verify createtree job is running in FIPS mode", func(ctx SpecContext) {
			p := createFipsTestPod(getNamespace(), "fips-createtree", "test-createtree", images.TrillianCreateTree,
				[]string{"/createtree"}, []string{"--admin_server=0.0.0.0:1", "--rpc_deadline=10m"}, godebug)
			Expect(cli.Create(ctx, p)).To(Succeed())
			DeferCleanup(func() { _ = cli.Delete(ctx, p) })
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(cli.Get(ctx, ctrlclient.ObjectKeyFromObject(p), p)).To(Succeed())
				g.Expect(p.Status.Phase).ToNot(Equal(v1.PodPending))
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, p, "test-createtree", getNamespace(), "/createtree")
		})

		It("Verify backfill-redis job is running in FIPS mode", func(ctx SpecContext) {
			p := createFipsTestPod(getNamespace(), "fips-backfill-redis", "backfill-redis", images.BackfillRedis,
				nil, []string{
					"--rekor-address=https://0.0.0.0:1",
					"--rekor-retry-count=20",
					"--redis-hostname=0.0.0.0",
					"--redis-port=1",
					"--start=0",
					"--end=0",
					"--concurrency=1",
				}, godebug)
			Expect(cli.Create(ctx, p)).To(Succeed())
			DeferCleanup(func() { _ = cli.Delete(ctx, p) })
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(cli.Get(ctx, ctrlclient.ObjectKeyFromObject(p), p)).To(Succeed())
				g.Expect(p.Status.Phase).ToNot(Equal(v1.PodPending))
			}).WithContext(ctx).Should(Succeed())
			verifyFipsGoNative(ctx, p, "backfill-redis", getNamespace(), "/usr/local/bin/backfill-redis")
		})

		// TODO: re-add tuf-init FIPS check when Go CLI replacement is available
	})
})

func createFipsTestPod(namespace, name, containerName string, image images.Image, command, args []string, envs ...v1.EnvVar) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			SecurityContext: &v1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				SeccompProfile: &v1.SeccompProfile{
					Type: v1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []v1.Container{
				{
					Name:    containerName,
					Image:   images.Registry.Get(image),
					Command: command,
					Args:    args,
					Env:     envs,
					SecurityContext: &v1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						RunAsNonRoot:             ptr.To(true),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
						SeccompProfile: &v1.SeccompProfile{
							Type: v1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
}

func verifyFipsGoNative(ctx SpecContext, pod *v1.Pod, containerName, namespace, expectedExe string) {
	verifyFipsKernel(ctx, pod, containerName, namespace)
	verifyGodebugOnly(ctx, pod, containerName, namespace)

	if expectedExe != "" {
		verifyFipsBinary(ctx, pod, containerName, namespace, expectedExe)

		out, err := k8ssupport.ExecInPodWithOutput(ctx,
			pod.Name, containerName, namespace,
			"grep", "-aom", "1", `GOFIPS140=`, expectedExe,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(strings.TrimSpace(string(out))).To(ContainSubstring("GOFIPS140="),
			fmt.Sprintf("binary %s should be built with GOFIPS140", expectedExe))
	}
}

func verifyFipsOpenSSL(ctx SpecContext, pod *v1.Pod, containerName, namespace, expectedExe string) {
	verifyFipsKernel(ctx, pod, containerName, namespace)
	verifyGodebugOnly(ctx, pod, containerName, namespace)
	verifyFipsBinary(ctx, pod, containerName, namespace, expectedExe)

	libcryptoMap, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"sh", "-c", `grep -E 'libcrypto\.so(\.3)?' /proc/1/maps | head -n 1`,
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(libcryptoMap))).To(ContainSubstring("libcrypto.so"))

	fips, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"sh", "-c", `grep -F 'ossl-modules/fips.so' /proc/1/maps | head -n 1`,
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(fips))).To(ContainSubstring("ossl-modules/fips.so"))
}

func verifyGodebugOnly(ctx SpecContext, pod *v1.Pod, containerName, namespace string) {
	out, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"printenv", "GODEBUG",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(out))).To(Equal("fips140=only"),
		fmt.Sprintf("container %s should have GODEBUG=fips140=only", containerName))
}

func verifyFipsKernel(ctx SpecContext, pod *v1.Pod, containerName, namespace string) {
	out, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"cat", "/proc/sys/crypto/fips_enabled",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(out))).To(Equal("1"))
}

func verifyFipsBinary(ctx SpecContext, pod *v1.Pod, containerName, namespace, expectedExe string) {
	if expectedExe == "" {
		return
	}
	exe, err := k8ssupport.ExecInPodWithOutput(ctx,
		pod.Name, containerName, namespace,
		"readlink", "-f", "/proc/1/exe",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(strings.TrimSpace(string(exe))).To(Equal(expectedExe))
}
