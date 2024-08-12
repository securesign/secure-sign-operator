package tuf

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/job"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tuf/actions"
	utils2 "github.com/securesign/operator/internal/controller/tuf/utils"
	appsv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Tuf) string {
			return meta.FindStatusCondition(f.Status.Conditions, constants.Ready).Reason
		}, Equal(constants.Ready)))

	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			kubernetes.ComponentLabel: actions.ComponentName,
		})
	}).Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Tuf {
	return func() *v1alpha1.Tuf {
		instance := &v1alpha1.Tuf{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func RefreshTufRepository(ctx context.Context, cli client.Client, ns string, name string) {
	tufDeployment := &appsv1.Deployment{}
	Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: actions.DeploymentName}, tufDeployment)).To(Succeed())

	// pause deployment reconciliation
	if tufDeployment.Annotations == nil {
		tufDeployment.Annotations = make(map[string]string)
	}
	tufDeployment.Annotations[annotations.PausedReconciliation] = "true"

	// scale deployment down to release PV
	tufDeployment.Spec.Replicas = ptr.To(int32(0))
	Expect(cli.Update(ctx, tufDeployment)).To(Succeed())

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
	Expect(cli.Get(ctx, types.NamespacedName{Namespace: ns, Name: actions.DeploymentName}, tufDeployment)).To(Succeed())
	tufDeployment.Annotations[annotations.PausedReconciliation] = "false"
	Expect(cli.Update(ctx, tufDeployment)).To(Succeed())

	// wait for controller to start loop again
	time.Sleep(5 * time.Second)
}

func refreshTufJob(instance *v1alpha1.Tuf) *v12.Job {
	return &v12.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "tuf-refresh-",
			Namespace:    instance.Namespace,
		},
		Spec: v12.JobSpec{
			Parallelism: ptr.To[int32](1),
			Completions: ptr.To[int32](1),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					ServiceAccountName: actions.RBACName,
					RestartPolicy:      v1.RestartPolicyNever,
					Volumes: []v1.Volume{
						{
							Name: "tuf-secrets",
							VolumeSource: v1.VolumeSource{
								Projected: utils2.SecretsVolumeProjection(instance.Status.Keys),
							},
						},
						{
							Name: "repository",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: instance.Status.PvcName,
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:  "tuf-init",
							Image: constants.TufImage,
							Env: []v1.EnvVar{
								{
									Name:  "NAMESPACE",
									Value: instance.Namespace,
								},
							},
							Args: []string{
								"-mode", "init",
								"-target-dir", "/var/run/target",
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "tuf-secrets",
									MountPath: "/var/run/tuf-secrets",
								},
								{
									Name:      "repository",
									MountPath: "/var/run/target",
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
	}
}
