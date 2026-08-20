package support

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestRegistry manages an OCI registry deployed in the Kubernetes cluster for e2e tests.
type TestRegistry struct {
	kubeClient kubernetes.Interface
	cliClient  client.Client
	namespace  string
	port       int32
	serviceName string
}

const (
	registryImage           = "registry:2.8"
	testRegistryServiceName = "test-registry"
)

// DeployTestRegistry deploys an OCI registry as a pod in the Kubernetes cluster.
// The registry is accessible via Kubernetes Service DNS from all pods in the cluster.
func DeployTestRegistry(ctx context.Context, cli interface{}, namespace string) *TestRegistry {
	var kubeClient kubernetes.Interface
	var cliClient client.Client

	// Handle different client types passed in
	switch c := cli.(type) {
	case kubernetes.Interface:
		kubeClient = c
	case client.Client:
		cliClient = c
	default:
		// If no valid client provided, skip deployment (might be running on host only)
	}

	port := int32(5000)

	// Deploy registry deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-registry",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-registry",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "registry",
							Image: registryImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "REGISTRY_HTTP_ADDR",
									Value: fmt.Sprintf("0.0.0.0:%d", port),
								},
								{
									Name:  "REGISTRY_STORAGE_DELETE_ENABLED",
									Value: "true",
								},
							},
						},
					},
				},
			},
		},
	}

	// Deploy the service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRegistryServiceName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "test-registry",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Create deployment if kubeClient is available
	if kubeClient != nil {
		_, err := kubeClient.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}

		// Create service
		_, err = kubeClient.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}
	}

	// Wait for deployment to be ready
	waitForDeploymentReady(ctx, kubeClient, namespace, "test-registry")

	core.GinkgoWriter.Printf("Test registry deployed in cluster at %s.%s.svc:5000\n", testRegistryServiceName, namespace)

	return &TestRegistry{
		kubeClient:  kubeClient,
		cliClient:   cliClient,
		namespace:   namespace,
		port:        port,
		serviceName: testRegistryServiceName,
	}
}

// PrepareImage creates a random OCI image and pushes it to the in-cluster registry.
// Returns the image reference accessible from the test runner (localhost:port/...).
// This uses port-forwarding or local access to push the image.
func (r *TestRegistry) PrepareImage(ctx context.Context) string {
	// Create a random image
	image, err := random.Image(1024, 8)
	Expect(err).ToNot(HaveOccurred())

	cfg, err := image.ConfigFile()
	Expect(err).ToNot(HaveOccurred())
	cfg.Config.Labels = map[string]string{
		"run-id": uuid.New().String(),
	}
	image, err = mutate.ConfigFile(image, cfg)
	Expect(err).ToNot(HaveOccurred())

	digest, err := image.Digest()
	Expect(err).ToNot(HaveOccurred())

	// For local pushing, use localhost (assumes port-forwarding is set up)
	imageRef := fmt.Sprintf("localhost:%d/e2e-test@%s", r.port, digest.String())
	ref, err := name.ParseReference(imageRef, name.Insecure)
	Expect(err).ToNot(HaveOccurred())

	pusher, err := remote.NewPusher()
	Expect(err).ToNot(HaveOccurred())
	Expect(pusher.Push(ctx, ref, image)).To(Succeed())

	core.GinkgoWriter.Printf("Pushed test image: %s\n", imageRef)
	return imageRef
}

// InClusterRef transforms the image reference to be accessible from within the cluster.
// Converts localhost references to the Kubernetes Service DNS name.
func (r *TestRegistry) InClusterRef(externalRef string) string {
	// Replace localhost with the service DNS name for cluster-internal access
	// Format: service-name.namespace.svc.cluster.local:port
	clusterDNS := fmt.Sprintf("%s.%s.svc", r.serviceName, r.namespace)
	return clusterDNS + ":" + externalRef[len("localhost:"):]
}

// Close cleans up the registry deployment and service.
func (r *TestRegistry) Close() {
	if r.kubeClient != nil {
		_ = r.kubeClient.AppsV1().Deployments(r.namespace).Delete(context.Background(), "test-registry", metav1.DeleteOptions{})
		_ = r.kubeClient.CoreV1().Services(r.namespace).Delete(context.Background(), testRegistryServiceName, metav1.DeleteOptions{})
	}
}

func ptr(v int32) *int32 {
	return &v
}

func waitForDeploymentReady(ctx context.Context, kubeClient kubernetes.Interface, namespace, name string) {
	if kubeClient == nil {
		return
	}

	maxAttempts := 60
	for i := 0; i < maxAttempts; i++ {
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil && deployment.Status.ReadyReplicas == 1 {
			core.GinkgoWriter.Printf("Deployment %s is ready\n", name)
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
			// Wait a bit before retrying
			<-time.After(time.Second)
		}
	}

	core.GinkgoWriter.Printf("Warning: Deployment %s failed to become ready\n", name)
}

// Keep the old function signature for backward compatibility but make it use the new approach
func DeployTestRegistryCompat(ctx context.Context, cli interface{}, namespace string) *TestRegistry {
	return DeployTestRegistry(ctx, cli, namespace)
}
