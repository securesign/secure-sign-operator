package steps

import (
	"context"
	"strings"
	"sync"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8ssupport "github.com/securesign/operator/test/e2e/support/kubernetes"
	olmhelpers "github.com/securesign/operator/test/e2e/support/kubernetes/olm"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	fipsOnce     sync.Once
	fipsDetected bool
	fipsPatched  bool
)

// IsFIPSCluster execs into the operator manager pod and checks
// /proc/sys/crypto/fips_enabled. Returns true if the host kernel has FIPS enabled.
// Uses sync.Once so detection happens at most once per test binary.
func IsFIPSCluster(ctx context.Context, cli client.Client) bool {
	fipsOnce.Do(func() {
		pod, containerName := FindOperatorPod(ctx, cli)
		if pod == nil {
			return
		}

		out, err := k8ssupport.ExecInPodWithOutput(ctx,
			pod.Name, containerName, pod.Namespace,
			"cat", "/proc/sys/crypto/fips_enabled",
		)
		if err != nil {
			return
		}
		fipsDetected = strings.TrimSpace(string(out)) == "1"
	})
	return fipsDetected
}

// DetectAndConfigureFIPS is a reusable BeforeAll step.
// It detects host FIPS mode by execing into the operator pod, and if enabled,
// patches the operator (via CSV if OLM-managed, otherwise directly) with
// GODEBUG=fips140=only and waits for the rollout to complete.
func DetectAndConfigureFIPS(cli client.Client, callback func(fipsEnabled bool)) func(ctx ginkgo.SpecContext) {
	return func(ctx ginkgo.SpecContext) {
		enabled := IsFIPSCluster(ctx, cli)
		if enabled && !fipsPatched {
			patchOperatorGodebug(ctx, cli)
			fipsPatched = true
		}
		callback(enabled)
	}
}

// FindOperatorPod returns the running operator pod and its first container name.
func FindOperatorPod(ctx context.Context, cli client.Client) (*v1.Pod, string) {
	list := &v1.PodList{}
	if err := cli.List(ctx, list,
		client.MatchingLabels{"control-plane": "operator-controller-manager"},
	); err != nil {
		return nil, ""
	}

	for i := range list.Items {
		pod := &list.Items[i]
		if !strings.Contains(pod.Spec.ServiceAccountName, "rhtas-operator") {
			continue
		}
		if pod.Status.Phase != v1.PodRunning {
			continue
		}
		if len(pod.Spec.Containers) == 0 {
			continue
		}
		return pod, pod.Spec.Containers[0].Name
	}
	return nil, ""
}

// FindOperatorDeployment returns the rhtas-operator deployment.
func FindOperatorDeployment(ctx context.Context, cli client.Client) *appsv1.Deployment {
	depList := &appsv1.DeploymentList{}
	Expect(cli.List(ctx, depList,
		client.MatchingLabels{"control-plane": "operator-controller-manager"},
	)).To(Succeed())
	Expect(depList.Items).ToNot(BeEmpty())

	for i := range depList.Items {
		dep := &depList.Items[i]
		if strings.Contains(dep.Spec.Template.Spec.ServiceAccountName, "rhtas-operator") {
			return dep
		}
	}
	Expect(false).To(BeTrue(), "could not find rhtas-operator deployment")
	return nil
}

// WaitForOperatorPodWithEnv waits until a running, ready, non-terminating operator
// pod has all the specified env vars on its first container.
func WaitForOperatorPodWithEnv(ctx context.Context, cli client.Client, envs ...v1.EnvVar) *v1.Pod {
	var mgr *v1.Pod
	Eventually(func(g Gomega, ctx context.Context) {
		list := &v1.PodList{}
		g.Expect(cli.List(ctx, list,
			client.MatchingLabels{"control-plane": "operator-controller-manager"},
		)).To(Succeed())
		g.Expect(list.Items).ToNot(BeEmpty())
		for i := range list.Items {
			pod := &list.Items[i]
			if !strings.Contains(pod.Spec.ServiceAccountName, "rhtas-operator") || pod.Status.Phase != v1.PodRunning {
				continue
			}
			if pod.DeletionTimestamp != nil {
				continue
			}
			ready := len(pod.Status.ContainerStatuses) > 0
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			if len(pod.Spec.Containers) == 0 {
				continue
			}
			matched := true
			for _, want := range envs {
				found := false
				for _, got := range pod.Spec.Containers[0].Env {
					if got.Name == want.Name && got.Value == want.Value {
						found = true
						break
					}
				}
				if !found {
					matched = false
					break
				}
			}
			if matched {
				mgr = pod
				return
			}
		}
		g.Expect(mgr).ToNot(BeNil(), "waiting for operator pod with required env vars")
	}).WithContext(ctx).Should(Succeed())
	return mgr
}

func patchOperatorGodebug(ctx context.Context, cli client.Client) {
	dep := FindOperatorDeployment(ctx, cli)
	godebug := v1.EnvVar{Name: "GODEBUG", Value: "fips140=only"}
	olmhelpers.PatchCSVDeploymentEnv(ctx, cli, dep.Namespace, dep.Name, dep.Spec.Template.Spec.Containers[0].Name, godebug)
	WaitForOperatorPodWithEnv(ctx, cli, godebug)
}
