package kubernetes

import (
	"context"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func PatchDeploymentEnv(ctx context.Context, cli ctrlclient.Client, namespace, deploymentName, containerName string, envs ...v1.EnvVar) {
	dep := &appsv1.Deployment{}
	Eventually(func(g Gomega) {
		g.Expect(cli.Get(ctx, types.NamespacedName{
			Name:      deploymentName,
			Namespace: namespace,
		}, dep)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())

	for i, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == containerName {
			for _, env := range envs {
				found := false
				for j, existing := range dep.Spec.Template.Spec.Containers[i].Env {
					if existing.Name == env.Name {
						dep.Spec.Template.Spec.Containers[i].Env[j] = env
						found = true
						break
					}
				}
				if !found {
					dep.Spec.Template.Spec.Containers[i].Env = append(dep.Spec.Template.Spec.Containers[i].Env, env)
				}
			}
			break
		}
	}

	Expect(cli.Update(ctx, dep)).To(Succeed())

	Eventually(func(g Gomega) {
		updated := &appsv1.Deployment{}
		g.Expect(cli.Get(ctx, types.NamespacedName{
			Name:      deploymentName,
			Namespace: namespace,
		}, updated)).To(Succeed())
		g.Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		g.Expect(updated.Status.UpdatedReplicas).To(Equal(*updated.Spec.Replicas))
		g.Expect(updated.Status.AvailableReplicas).To(Equal(*updated.Spec.Replicas))

		for _, c := range updated.Spec.Template.Spec.Containers {
			if c.Name == containerName {
				for _, want := range envs {
					found := false
					for _, got := range c.Env {
						if got.Name == want.Name && got.Value == want.Value {
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "env %s=%s not found on container %s in deployment %s", want.Name, want.Value, containerName, deploymentName)
				}
				return
			}
		}
		g.Expect(false).To(BeTrue(), "container %s not found in deployment %s", containerName, deploymentName)
	}).WithContext(ctx).Should(Succeed())
}
