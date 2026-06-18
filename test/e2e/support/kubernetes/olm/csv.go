package olm

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	coreV1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8ssupport "github.com/securesign/operator/test/e2e/support/kubernetes"
)

// PatchCSVDeploymentEnv patches environment variables on a deployment's operator.
// If OLM manages the deployment (a CSV owns it), the CSV is patched so OLM
// reconciles the change. Otherwise the deployment is patched directly.
func PatchCSVDeploymentEnv(ctx context.Context, cli client.Client, namespace, deploymentName, containerName string, envs ...coreV1.EnvVar) {
	utilruntime.Must(v1alpha1.AddToScheme(cli.Scheme()))

	// Check if any CSV owns this deployment. If not, fall back to direct patch.
	csvList := &v1alpha1.ClusterServiceVersionList{}
	if err := cli.List(ctx, csvList, client.InNamespace(namespace)); err != nil || !csvOwnsDeployment(csvList, deploymentName, containerName) {
		k8ssupport.PatchDeploymentEnv(ctx, cli, namespace, deploymentName, containerName, envs...)
		return
	}

	Eventually(func(g Gomega, ctx context.Context) {
		list := &v1alpha1.ClusterServiceVersionList{}
		g.Expect(cli.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		for i := range list.Items {
			csv := &list.Items[i]
			if patchCSVContainerEnv(csv, deploymentName, containerName, envs...) {
				g.Expect(cli.Update(ctx, csv)).To(Succeed())
				return
			}
		}
		g.Expect(false).To(BeTrue(), "CSV with deployment %s not found in namespace %s", deploymentName, namespace)
	}).WithContext(ctx).Should(Succeed())

	k8ssupport.WaitForDeploymentRollout(ctx, cli, namespace, deploymentName, containerName, envs...)
}

func csvOwnsDeployment(csvList *v1alpha1.ClusterServiceVersionList, deploymentName, containerName string) bool {
	for i := range csvList.Items {
		for _, dep := range csvList.Items[i].Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
			if dep.Name != deploymentName {
				continue
			}
			for _, c := range dep.Spec.Template.Spec.Containers {
				if c.Name == containerName {
					return true
				}
			}
		}
	}
	return false
}

func patchCSVContainerEnv(csv *v1alpha1.ClusterServiceVersion, deploymentName, containerName string, envs ...coreV1.EnvVar) bool {
	for i, dep := range csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if dep.Name != deploymentName {
			continue
		}
		for j, c := range dep.Spec.Template.Spec.Containers {
			if c.Name != containerName {
				continue
			}
			for _, env := range envs {
				found := false
				for k, existing := range csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs[i].Spec.Template.Spec.Containers[j].Env {
					if existing.Name == env.Name {
						csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs[i].Spec.Template.Spec.Containers[j].Env[k] = env
						found = true
						break
					}
				}
				if !found {
					csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs[i].Spec.Template.Spec.Containers[j].Env = append(
						csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs[i].Spec.Template.Spec.Containers[j].Env, env)
				}
			}
			return true
		}
	}
	return false
}
