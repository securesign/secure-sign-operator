package cosign

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/utils/kubernetes/job"
	"github.com/securesign/operator/test/e2e/support"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const cosignImage = "registry.redhat.io/rhtas/cosign-rhel9:1.4.0"

type InClusterCosign struct {
	tufUrl    string
	namespace string
	cli       client.Client
}

func NewInClusterCosign(namespace, tufUrl string, client client.Client) *InClusterCosign {
	return &InClusterCosign{
		tufUrl:    tufUrl,
		namespace: namespace,
		cli:       client,
	}
}

func (c *InClusterCosign) Sign(ctx context.Context, targetImageName string) error {
	oidcToken, err := support.OidcToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OIDC token: %w", err)
	}
	if oidcToken == "" {
		return fmt.Errorf("received empty OIDC token")
	}

	return c.executeInJob(ctx, fmt.Sprintf("cosign sign -y --identity-token=%s %s", oidcToken, targetImageName))
}

func (c *InClusterCosign) Verify(ctx context.Context, targetImageName string) error {
	return c.executeInJob(ctx, fmt.Sprintf("cosign verify --certificate-identity-regexp '.*@redhat' --certificate-oidc-issuer-regexp '.*keycloak.*' %s", targetImageName))
}

func (c *InClusterCosign) VerifyByCosign(ctx context.Context, targetImageName string) {

	Eventually(func(g Gomega, ctx context.Context) error {
		oidcToken, err := support.OidcToken(ctx)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(oidcToken).ToNot(BeEmpty())
		return c.executeInJob(ctx, fmt.Sprintf(`cosign initialize --mirror=%s --root=%s/root.json \
		&& cosign sign -y --identity-token=%s %s \
		&& cosign verify --certificate-identity-regexp '.*@redhat' --certificate-oidc-issuer-regexp '.*keycloak.*' %s
		`, c.tufUrl, c.tufUrl, oidcToken, targetImageName, targetImageName))
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
}

func (c *InClusterCosign) executeInJob(ctx context.Context, script string) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "cosign-",
			Namespace:    c.namespace,
		},
		Spec: batchv1.JobSpec{
			Parallelism:           ptr.To[int32](1),
			Completions:           ptr.To[int32](1),
			BackoffLimit:          ptr.To(int32(0)),
			ActiveDeadlineSeconds: ptr.To(int64(60)),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []v1.Container{
						{
							Name:    "cosign",
							Image:   cosignImage,
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{script},
						},
					},
				},
			},
		},
	}
	err := c.cli.Create(ctx, j)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return wait.PollUntilContextTimeout(ctx, 1*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		current, err := job.GetJob(ctx, c.cli, c.namespace, j.Name)
		if err != nil {
			return false, err
		}
		if job.IsCompleted(*current) {

			if job.IsFailed(*current) {
				return false, fmt.Errorf("job %s failed", current.Name)
			}

			return true, nil
		}
		return false, nil
	})
}
