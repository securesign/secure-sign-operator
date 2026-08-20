package support

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
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

// TestRegistry manages an OCI registry for e2e tests.
// It automatically chooses between in-memory (host-based) or in-cluster deployment.
type TestRegistry struct {
	// Common fields
	namespace   string
	port        uint16
	serviceName string

	// In-memory registry fields (host-based)
	server   *http.Server
	listener net.Listener
	hostIP   string

	// In-cluster registry fields
	kubeClient kubernetes.Interface
	cliClient  client.Client
}

const (
	registryImage           = "registry:2.8"
	testRegistryServiceName = "test-registry"
)

// DeployTestRegistry deploys an OCI registry, choosing the best strategy for the environment.
// - If running inside a cluster: deploys as a Kubernetes pod with Service DNS
// - If running outside a cluster: runs in-memory on the host with gateway IP for pod access
func DeployTestRegistry(ctx context.Context, cli interface{}, namespace string) *TestRegistry {
	// Detect if we're running inside a Kubernetes cluster
	if isInsideCluster() {
		return deployInClusterRegistry(ctx, cli, namespace)
	}
	return deployHostRegistry(ctx)
}

// deployHostRegistry creates an in-memory registry on the host machine.
// Pods access it via the Docker gateway IP (172.17.0.1 or discovered automatically).
func deployHostRegistry(ctx context.Context) *TestRegistry {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())

	addr := listener.Addr().(*net.TCPAddr)
	port := uint16(addr.Port)

	handler := registry.New()

	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			core.GinkgoWriter.Printf("registry server error: %v\n", err)
		}
	}()

	// Discover the host IP that's accessible from Docker containers
	hostIP := getDockerHostIP()

	core.GinkgoWriter.Printf("In-memory registry started on localhost:%d (accessible from pods as %s:%d)\n", port, hostIP, port)

	return &TestRegistry{
		server:   server,
		listener: listener,
		port:     port,
		hostIP:   hostIP,
	}
}

// deployInClusterRegistry deploys the registry as a Kubernetes pod with a Service.
func deployInClusterRegistry(ctx context.Context, cli interface{}, namespace string) *TestRegistry {
	var kubeClient kubernetes.Interface

	// Try to get kubernetes client
	switch c := cli.(type) {
	case kubernetes.Interface:
		kubeClient = c
	case client.Client:
		// If we got a controller-runtime client, we can use it directly
		// But for Kubernetes operations, we need the clientset
		// This is a limitation - ideally we'd convert it
	}

	port := int32(5000)

	// Create deployment
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

	// Create service
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

	// Create resources if kubeClient is available
	if kubeClient != nil {
		_, err := kubeClient.AppsV1().Deployments(namespace).Create(context.Background(), deployment, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}

		_, err = kubeClient.CoreV1().Services(namespace).Create(context.Background(), service, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).ToNot(HaveOccurred())
		}

		// Wait for deployment
		waitForDeploymentReady(context.Background(), kubeClient, namespace, "test-registry")
	}

	core.GinkgoWriter.Printf("In-cluster registry deployed at %s.%s.svc:5000\n", testRegistryServiceName, namespace)

	return &TestRegistry{
		kubeClient:  kubeClient,
		namespace:   namespace,
		port:        5000,
		serviceName: testRegistryServiceName,
	}
}

// PrepareImage creates a random OCI image and pushes it to the registry.
// Returns the image reference in the format expected by the environment.
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

	// For localhost (in-memory registry), use localhost:port
	imageRef := fmt.Sprintf("localhost:%d/e2e-test@%s", r.port, digest.String())
	ref, err := name.ParseReference(imageRef, name.Insecure)
	Expect(err).ToNot(HaveOccurred())

	pusher, err := remote.NewPusher()
	Expect(err).ToNot(HaveOccurred())
	Expect(pusher.Push(ctx, ref, image)).To(Succeed())

	core.GinkgoWriter.Printf("Pushed test image: %s\n", imageRef)
	return imageRef
}

// InClusterRef transforms the image reference to be accessible from pods.
// - For host-based registry: converts localhost to the Docker gateway IP
// - For in-cluster registry: converts localhost to the service DNS name
func (r *TestRegistry) InClusterRef(externalRef string) string {
	if r.server != nil {
		// Host-based registry: use the host IP that pods can reach
		return r.hostIP + ":" + externalRef[len("localhost:"):]
	}

	// In-cluster registry: use service DNS
	clusterDNS := fmt.Sprintf("%s.%s.svc", r.serviceName, r.namespace)
	return clusterDNS + ":" + externalRef[len("localhost:"):]
}

// Close cleans up the registry.
func (r *TestRegistry) Close() {
	if r.server != nil {
		// Host-based registry
		_ = r.server.Close()
		return
	}

	// In-cluster registry
	if r.kubeClient != nil {
		_ = r.kubeClient.AppsV1().Deployments(r.namespace).Delete(context.Background(), "test-registry", metav1.DeleteOptions{})
		_ = r.kubeClient.CoreV1().Services(r.namespace).Delete(context.Background(), testRegistryServiceName, metav1.DeleteOptions{})
	}
}

// Helper functions

func ptr(v int32) *int32 {
	return &v
}

// isInsideCluster checks if the code is running inside a Kubernetes pod.
func isInsideCluster() bool {
	// If KUBERNETES_SERVICE_HOST is set, we're running in a pod
	_, exists := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return exists
}

// getDockerHostIP discovers the host IP that's accessible from Docker containers.
// For Kind/Docker Desktop, this is typically 172.17.0.1 or the Docker gateway.
func getDockerHostIP() string {
	// First try common Docker gateway IPs
	candidates := []string{
		"172.17.0.1",     // Default Docker gateway
		"172.18.0.1",     // Alternative Docker gateway
		"192.168.65.1",   // Docker Desktop on Mac
		"host.docker.internal", // Docker Desktop hostname
	}

	for _, ip := range candidates {
		if isReachable(ip, 5000, 1*time.Second) {
			return ip
		}
	}

	// Fallback: try to detect from the local machine's IP
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		if localAddr != nil && localAddr.IP != nil {
			return localAddr.IP.String()
		}
	}

	// Last resort: localhost (won't work for pods but better than nothing)
	core.GinkgoWriter.Printf("Warning: Could not determine Docker host IP, using localhost\n")
	return "localhost"
}

// isReachable checks if a host:port is reachable with a timeout.
func isReachable(host string, port uint16, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
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
			time.Sleep(time.Second)
		}
	}

	core.GinkgoWriter.Printf("Warning: Deployment %s failed to become ready\n", name)
}
