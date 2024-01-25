package tas

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/ctlog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func VerifyCTLog(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(func() v1alpha1.Phase {
		instance := &v1alpha1.CTlog{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, instance)
		return instance.Status.Phase
	}).Should(Equal(v1alpha1.PhaseReady), "Failed to verify CTLog deployment")

	list := &v1.PodList{}
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: ctlog.ComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))), "Failed to verify CTLog pod")
	// If verification fails, print the CTLog Deployment YAML

	PrintCTLogDeploymentYAML(ctx, cli, namespace, name)
	PrintEvents(ctx, cli, namespace)
}

func CurrentGinkgoTestDescription() {
	panic("unimplemented")
}

func GetCTLogServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: ctlog.ComponentName, kubernetes.NameLabel: "ctlog"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func PrintCTLogDeploymentYAML(ctx context.Context, cli client.Client, namespace, name string) {
	instance := &v1alpha1.CTlog{}
	err := cli.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, instance)

	if err != nil {
		fmt.Printf("Error getting CTLog deployment: %v\n", err)
		return
	}

	yamlData, err := yaml.Marshal(instance)
	if err != nil {
		fmt.Printf("Error marshaling CTLog deployment to YAML: %v\n", err)
		return
	}

	fmt.Println("CTLog Deployment YAML:")
	fmt.Println(strings.TrimSpace(string(yamlData)))
}

// function to print events in the namespace
func PrintEvents(ctx context.Context, cli client.Client, namespace string) {
	list := &v1.EventList{}
	cli.List(ctx, list, client.InNamespace(namespace))
	for _, e := range list.Items {
		fmt.Printf("Event: %s\n", e.Name)
	}
}
