package tuf

import (
	"context"
	"maps"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	utils2 "github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/test/e2e/support/condition"
	appsv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		And(
			Not(BeNil()),
			WithTransform(condition.IsReady, BeTrue()),
		))

	Eventually(condition.DeploymentIsRunning(ctx, cli, namespace, constants.ComponentName)).
		Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Tuf {
	return func() *v1alpha1.Tuf {
		instance := &v1alpha1.Tuf{}
		if e := cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance); errors.IsNotFound(e) {
			return nil
		}
		return instance
	}
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: constants.ComponentName})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func RefreshTufRepository(ctx context.Context, cli client.Client, ns string, name string) {
	tufDeployment := &appsv1.Deployment{}
	Eventually(func(g Gomega) error {
		g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: constants.DeploymentName}, tufDeployment)).To(Succeed())

		// pause deployment reconciliation
		if tufDeployment.Annotations == nil {
			tufDeployment.Annotations = make(map[string]string)
		}
		tufDeployment.Annotations[annotations.PausedReconciliation] = "true"

		// scale deployment down to release PV
		tufDeployment.Spec.Replicas = ptr.To(int32(0))
		return cli.Update(ctx, tufDeployment)
	}).WithTimeout(1 * time.Second).Should(Succeed())

	t := Get(ctx, cli, ns, name)()
	Expect(t).ToNot(BeNil())
	refreshJob := refreshTufJob(t)
	Expect(cli.Create(ctx, refreshJob)).To(Succeed())

	Eventually(func(g Gomega) bool {
		found := &v12.Job{}
		g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(refreshJob), found)).To(Succeed())
		return job.IsCompleted(*found) && !job.IsFailed(*found)
	}).Should(BeTrue())

	// unpause reconciliation
	Eventually(func(g Gomega) error {
		g.Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: constants.DeploymentName}, tufDeployment)).To(Succeed())
		tufDeployment.Annotations[annotations.PausedReconciliation] = "false"
		return cli.Update(ctx, tufDeployment)
	},
	).WithTimeout(1 * time.Second).Should(Succeed())

	// wait for controller to start loop again
	time.Sleep(5 * time.Second)
}

func refreshTufJob(instance *v1alpha1.Tuf) *v12.Job {
	j := &v12.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    instance.Namespace,
			GenerateName: "tuf-refresh-",
		},
	}
	l := maps.Clone(instance.Labels)
	l[labels.LabelAppComponent] = "test"
	Expect(utils2.EnsureTufInitJob(instance, constants.RBACName, instance.Labels)(j)).To(Succeed())
	c := kubernetes.FindContainerByNameOrCreate(&j.Spec.Template.Spec, "tuf-init")
	c.Command = []string{"/bin/sh", "-c"}
	args := c.Args
	c.Args = []string{"rm -rf /var/run/target/* && /usr/bin/tuf-repo-init.sh " + strings.Join(args, " ")}
	return j
}
