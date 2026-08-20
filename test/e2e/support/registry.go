package support

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	containerv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

// TestRegistry manages an in-memory OCI registry for e2e tests.
type TestRegistry struct {
	server   *http.Server
	listener net.Listener
	port     uint16
}

// DeployTestRegistry creates an in-memory OCI registry and starts an HTTP server.
// The ctx, cli, and namespace parameters are kept for backward compatibility but unused.
func DeployTestRegistry(ctx context.Context, _ interface{}, _ string) *TestRegistry {
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

	core.GinkgoWriter.Printf("In-memory registry started on localhost:%d\n", port)

	return &TestRegistry{
		server:   server,
		listener: listener,
		port:     port,
	}
}

// PrepareImage creates a random OCI image and pushes it to the in-memory registry.
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

	imageRef := fmt.Sprintf("localhost:%d/e2e-test@%s", r.port, digest.String())
	ref, err := name.ParseReference(imageRef, name.Insecure)
	Expect(err).ToNot(HaveOccurred())

	pusher, err := remote.NewPusher()
	Expect(err).ToNot(HaveOccurred())
	Expect(pusher.Push(ctx, ref, image)).To(Succeed())

	core.GinkgoWriter.Printf("Pushed test image: %s\n", imageRef)
	return imageRef
}

// InClusterRef returns the image reference unchanged since everything runs in-process.
func (r *TestRegistry) InClusterRef(externalRef string) string {
	return externalRef
}

// Close shuts down the HTTP server.
func (r *TestRegistry) Close() {
	if r.server != nil {
		_ = r.server.Close()
	}
}
