package support

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	containerv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	registryImage = "docker.io/library/registry:2"
	registryName  = "test-registry"
	registryPort  = int32(5000)
)

// TestRegistry manages an in-cluster OCI registry for e2e tests,
// eliminating the need for external registries like quay.io.
type TestRegistry struct {
	namespace string
	cli       client.Client
	localPort uint16
	stopCh    chan struct{}
}

// DeployTestRegistry creates a registry Deployment + Service in the given namespace,
// waits for readiness, and sets up port-forwarding for access from the test runner.
func DeployTestRegistry(ctx context.Context, cli client.Client, namespace string) *TestRegistry {
	r := &TestRegistry{
		namespace: namespace,
		cli:       cli,
	}

	labels := map[string]string{"app": registryName}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registryName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "registry",
							Image: registryImage,
							Ports: []corev1.ContainerPort{{ContainerPort: registryPort}},
							Env: []corev1.EnvVar{
								{Name: "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", Value: "/tmp/registry"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/tmp/registry"},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								RunAsUser:                ptr.To(int64(1000)),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name:         "data",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
	}
	Expect(cli.Create(ctx, deployment)).To(Succeed())

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      registryName,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Port: registryPort, TargetPort: intstr.FromInt32(registryPort)},
			},
		},
	}
	Expect(cli.Create(ctx, service)).To(Succeed())

	core.GinkgoWriter.Println("Waiting for test registry to be ready...")
	Eventually(func(g Gomega) {
		d := &appsv1.Deployment{}
		g.Expect(cli.Get(ctx, client.ObjectKeyFromObject(deployment), d)).To(Succeed())
		g.Expect(d.Status.ReadyReplicas).To(Equal(int32(1)))
	}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

	r.startPortForward(ctx)
	return r
}

func (r *TestRegistry) startPortForward(ctx context.Context) {
	restConfig, err := config.GetConfig()
	Expect(err).ToNot(HaveOccurred())

	clientset, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred())

	podList, err := clientset.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + registryName,
		FieldSelector: "status.phase=Running",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(podList.Items).ToNot(BeEmpty(), "no running registry pod found")

	podName := podList.Items[0].Name

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	Expect(err).ToNot(HaveOccurred())

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(r.namespace).
		Name(podName).
		SubResource("portforward")

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	r.stopCh = make(chan struct{})
	readyCh := make(chan struct{})

	fw, err := portforward.New(dialer, []string{"0:5000"}, r.stopCh, readyCh, io.Discard, io.Discard)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		if fwErr := fw.ForwardPorts(); fwErr != nil {
			core.GinkgoWriter.Printf("port-forward error: %v\n", fwErr)
		}
	}()

	<-readyCh

	ports, err := fw.GetPorts()
	Expect(err).ToNot(HaveOccurred())
	r.localPort = ports[0].Local

	core.GinkgoWriter.Printf("Port-forwarding %s:%d -> localhost:%d\n", registryName, registryPort, r.localPort)
}

// PrepareImage creates a random OCI image and pushes it to the in-cluster registry.
// Returns the image reference accessible from the test runner (localhost:port/...).
func (r *TestRegistry) PrepareImage(ctx context.Context) string {
	if v, ok := os.LookupEnv("TEST_IMAGE"); ok {
		return v
	}

	image, err := random.Image(1024, 8)
	Expect(err).ToNot(HaveOccurred())

	image, err = mutate.Config(image, containerv1.Config{
		Labels: map[string]string{
			"run-id": uuid.New().String(),
		},
	})
	Expect(err).ToNot(HaveOccurred())

	digest, err := image.Digest()
	Expect(err).ToNot(HaveOccurred())

	imageRef := fmt.Sprintf("localhost:%d/e2e-test@%s", r.localPort, digest.String())
	ref, err := name.ParseReference(imageRef, name.Insecure)
	Expect(err).ToNot(HaveOccurred())

	pusher, err := remote.NewPusher()
	Expect(err).ToNot(HaveOccurred())
	Expect(pusher.Push(ctx, ref, image)).To(Succeed())

	core.GinkgoWriter.Printf("Pushed test image: %s\n", imageRef)
	return imageRef
}

// InClusterRef converts an external (localhost) image reference to one
// accessible from within the cluster via the registry's Service DNS name.
func (r *TestRegistry) InClusterRef(externalRef string) string {
	localPrefix := fmt.Sprintf("localhost:%d/", r.localPort)
	inClusterPrefix := fmt.Sprintf("%s.%s.svc:%d/", registryName, r.namespace, registryPort)
	return strings.Replace(externalRef, localPrefix, inClusterPrefix, 1)
}

// Close tears down the port-forward connection.
func (r *TestRegistry) Close() {
	if r.stopCh != nil {
		close(r.stopCh)
	}
}
