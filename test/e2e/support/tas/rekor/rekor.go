package rekor

import (
	"context"

	"github.com/securesign/operator/test/e2e/support"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Rekor) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	// server
	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			constants.LabelAppComponent: actions.ServerComponentName,
		})
	}).Should(BeTrue())

	// redis
	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			constants.LabelAppComponent: actions.RedisComponentName,
		})
	}).Should(BeTrue())
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{constants.LabelAppComponent: actions.ServerComponentName, constants.LabelAppName: "rekor-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Rekor {
	return func() *v1alpha1.Rekor {
		instance := &v1alpha1.Rekor{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
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
