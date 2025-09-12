package rekor

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string, db bool) {
	Eventually(Get).WithContext(ctx).WithArguments(cli, namespace, name).
		Should(
			And(
				Not(BeNil()),
				WithTransform(condition.IsReady, BeTrue()),
			))

	// server
	Eventually(condition.DeploymentIsRunning).WithContext(ctx).
		WithArguments(cli, namespace, actions.ServerComponentName).
		Should(BeTrue())

	if db {
		// redis
		Eventually(condition.DeploymentIsRunning).WithContext(ctx).
			WithArguments(cli, namespace, actions.RedisComponentName).
			Should(BeTrue())
	}
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) *v1.Pod {
	list := &v1.PodList{}
	_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: actions.ServerComponentName, labels.LabelAppName: "rekor-server"})
	if len(list.Items) != 1 {
		return nil
	}
	return &list.Items[0]
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *v1alpha1.Rekor {
	instance := &v1alpha1.Rekor{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance
}

func CreateSecret(ns string, name string) *v1.Secret {
	public, private, _, err := support.CreateCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

func SetRekorReplicaCount(ctx context.Context, cli client.Client, namespace, securesignDeploymentName string, replicaCount int32) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		s := securesign.Get(ctx, cli, namespace, securesignDeploymentName)
		Expect(s).ToNot(BeNil())

		s.Spec.Rekor.Replicas = &replicaCount
		return cli.Update(ctx, s)
	})
	Expect(err).ToNot(HaveOccurred())

	Eventually(func(g Gomega, ctx context.Context) {
		rk := securesign.Get(ctx, cli, namespace, securesignDeploymentName)
		g.Expect(rk).ToNot(BeNil())
		g.Expect(rk.Spec.Rekor.Replicas).To(Equal(ptr.To(replicaCount)))
	}).WithContext(ctx).Should(Succeed())
}
