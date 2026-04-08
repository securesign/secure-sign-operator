package apis

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"k8s.io/utils/ptr"
)

func TestResolveServiceAddress_WithoutProtocol(t *testing.T) {
	g := NewWithT(t)
	service := &v1alpha1.FulcioService{
		Address: "path.org/test",
		Port:    ptr.To(int32(8080)),
	}
	url, err := ServiceAsUrl(service)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(url).To(Equal("path.org:8080/test"))
}

func TestResolveServiceAddress_WithProtocol(t *testing.T) {
	g := NewWithT(t)
	service := &v1alpha1.FulcioService{
		Address: "https://path.org/test",
		Port:    ptr.To(int32(8080)),
	}
	url, err := ServiceAsUrl(service)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(url).To(Equal("https://path.org:8080/test"))
}

func TestResolveServiceAddress_WithoutPort(t *testing.T) {
	g := NewWithT(t)
	service := &v1alpha1.FulcioService{
		Address: "path.org/test",
	}
	url, err := ServiceAsUrl(service)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(url).To(Equal("path.org/test"))
}
