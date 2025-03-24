package ctlog

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		And(
			Not(BeNil()),
			WithTransform(condition.IsReady, BeTrue()),
		))

	Eventually(condition.DeploymentIsRunning(ctx, cli, namespace, actions.ComponentName)).
		Should(BeTrue())
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: actions.ComponentName, labels.LabelAppName: "ctlog"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.CTlog {
	return func() *v1alpha1.CTlog {
		instance := &v1alpha1.CTlog{}
		if e := cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance); errors.IsNotFound(e) {
			return nil
		}
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
