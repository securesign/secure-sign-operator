package tuf

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	utils2 "github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	"github.com/securesign/operator/test/e2e/support/condition"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	appsv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	batchv1ac "k8s.io/client-go/applyconfigurations/batch/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get).WithContext(ctx).
		WithArguments(cli, namespace, name).
		Should(
			And(
				Not(BeNil()),
				WithTransform(condition.IsReady, BeTrue()),
			))

	Eventually(condition.DeploymentIsRunning).WithContext(ctx).
		WithArguments(cli, namespace, constants.ComponentName).
		Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *rhtasv1.Tuf {
	instance := &rhtasv1.Tuf{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance

}

func GetServerPod(ctx context.Context, cli client.Client, ns string) *v1.Pod {
	list := &v1.PodList{}
	_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: constants.ComponentName})
	for _, pod := range list.Items {
		if _, hasLabel := pod.Labels["batch.kubernetes.io/job-name"]; !hasLabel {
			return &pod
		}
	}
	return nil
}

func RefreshTufRepository(ctx context.Context, cli client.Client, ns string, name string) {
	tufDeployment := &appsv1.Deployment{}
	Eventually(func(g Gomega, ctx context.Context) error {
		g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: constants.DeploymentName}, tufDeployment)).To(Succeed())

		// pause deployment reconciliation
		if tufDeployment.Annotations == nil {
			tufDeployment.Annotations = make(map[string]string)
		}
		tufDeployment.Annotations[annotations.PausedReconciliation] = "true"

		// scale deployment down to release PV
		tufDeployment.Spec.Replicas = ptr.To(int32(0))
		return cli.Update(ctx, tufDeployment)
	}).WithContext(ctx).WithTimeout(1 * time.Second).Should(Succeed())

	t := Get(ctx, cli, ns, name)
	Expect(t).ToNot(BeNil())

	if len(t.Spec.Ctlog) == 0 {
		t.Spec.Ctlog = []rhtasv1.TrustRootBinding{{}}
	}
	t.Spec.Ctlog[0].URL = ctlog.Get(ctx, cli, ns, name).Status.URL

	if len(t.Spec.Fulcio) == 0 {
		t.Spec.Fulcio = []rhtasv1.TrustRootBindingWithOIDC{{}}
	}
	t.Spec.Fulcio[0].URL = fulcio.Get(ctx, cli, ns, name).Status.URL

	if len(t.Spec.Rekor) == 0 {
		t.Spec.Rekor = []rhtasv1.TrustRootBinding{{}}
	}
	t.Spec.Rekor[0].URL = rekor.Get(ctx, cli, ns, name).Status.URL

	if t.Spec.Tsa == nil || len(*t.Spec.Tsa) == 0 {
		t.Spec.Tsa = &[]rhtasv1.TrustRootBinding{{}}
	}
	(*t.Spec.Tsa)[0].URL = tsa.Get(ctx, cli, ns, name).Status.URL
	refreshJob := refreshTufJob(ctx, cli, t)
	Expect(kubernetes.Create(ctx, cli, refreshJob)).To(Succeed())

	Eventually(func(g Gomega, ctx context.Context) bool {
		found := &v12.Job{}
		g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(refreshJob), found)).To(Succeed())
		return job.IsCompleted(*found) && !job.IsFailed(*found)
	}).WithContext(ctx).Should(BeTrue())

	// unpause reconciliation
	Eventually(func(g Gomega, ctx context.Context) error {
		g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: constants.DeploymentName}, tufDeployment)).To(Succeed())
		tufDeployment.Annotations[annotations.PausedReconciliation] = "false"
		return cli.Update(ctx, tufDeployment)
	},
	).WithContext(ctx).WithTimeout(1 * time.Second).Should(Succeed())

	// wait for controller to start loop again
	time.Sleep(5 * time.Second)
}

func refreshTufJob(ctx context.Context, cli client.Client, instance *rhtasv1.Tuf) *v12.Job {
	l := maps.Clone(instance.Labels)
	l[labels.LabelAppComponent] = "test"
	jobConfig, err := traits.Apply(
		batchv1ac.Job("", instance.Namespace).WithGenerateName("tuf-refresh-"),
		traits.Labels[*batchv1ac.JobApplyConfiguration](l),
		utils2.EnsureTufInitJob(ctx, cli, instance),
		traits.WithJobPodSpecTraits(
			traits.SecurityContexts(utils2.TufInitContainerName),
			traits.ServiceAccount(constants.RBACInitJobName),
			func(podSpec *corev1ac.PodSpecApplyConfiguration) error {
				container := kubernetes.FindContainerByName(podSpec, utils2.TufInitContainerName)
				if container == nil {
					return fmt.Errorf("TUF init container is not configured")
				}
				container.Args = []string{"rm -rf /var/run/target/* && " + strings.Join(container.Args, " ")}
				return nil
			},
		),
	)
	Expect(err).To(Succeed())
	refreshJob, err := traits.ConvertViaJSON[*batchv1ac.JobApplyConfiguration, v12.Job](jobConfig)
	Expect(err).To(Succeed())
	return refreshJob
}

func SetTufReplicaCount(ctx context.Context, cli client.Client, namespace, securesignDeploymentName string, replicaCount int32) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		s := securesign.Get(ctx, cli, namespace, securesignDeploymentName)
		Expect(s).ToNot(BeNil())

		s.Spec.Tuf.Replicas = &replicaCount
		return cli.Update(ctx, s)
	})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func(g Gomega, ctx context.Context) {
		tf := securesign.Get(ctx, cli, namespace, securesignDeploymentName)
		g.Expect(tf).ToNot(BeNil())
		g.Expect(tf.Spec.Tuf.Replicas).To(Equal(ptr.To(replicaCount)))
	}).WithContext(ctx).Should(Succeed())
}
