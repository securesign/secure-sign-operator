package utils

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	tsa "github.com/securesign/operator/internal/controller/tsa/actions"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	ctx = context.TODO()
	c   = fake.NewFakeClient()
)

func TestResolveServiceAddress_UserSpecifiedAddress(t *testing.T) {
	g := NewWithT(t)
	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testNamespace",
		},
		Spec: v1alpha1.TufSpec{
			Rekor: v1alpha1.RekorService{
				Address: "http://rekor.fakeserver.com",
			},
			Ctlog: v1alpha1.CtlogService{
				Address: "http://ctlog.fakeserver.com",
			},
			Fulcio: v1alpha1.FulcioService{
				Address: "http://fulcio.fakeserver.com",
			},
			Tsa: v1alpha1.TsaService{
				Address: "http://tsa.fakeserver.com",
			},
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "ctfe.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
				{
					Name: "tsa.certchain.pem",
				},
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLExternal,
		},
	}
	err := ResolveServiceAddress(ctx, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Spec.Rekor.Address).To(Equal("http://rekor.fakeserver.com"))
	g.Expect(instance.Spec.Ctlog.Address).To(Equal("http://ctlog.fakeserver.com"))
	g.Expect(instance.Spec.Fulcio.Address).To(Equal("http://fulcio.fakeserver.com"))
	// do not append the timestamp path to the user-provided address
	g.Expect(instance.Spec.Tsa.Address).To(Equal("http://tsa.fakeserver.com"))
}

func TestResolveServiceAddress_Internal(t *testing.T) {
	g := NewWithT(t)
	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testNamespace",
		},
		Spec: v1alpha1.TufSpec{
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "ctfe.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLInternal,
		},
	}
	err := ResolveServiceAddress(ctx, c, instance)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instance.Spec.Rekor.Address).To(Equal("http://rekor-server.testNamespace.svc"))
	g.Expect(instance.Spec.Ctlog.Address).To(Equal("http://ctlog.testNamespace.svc"))
	g.Expect(instance.Spec.Fulcio.Address).To(Equal("http://fulcio-server.testNamespace.svc"))
	g.Expect(instance.Spec.Tsa.Address).To(BeEmpty())
}

func TestResolveServiceAddress_External(t *testing.T) {
	g := NewWithT(t)

	for _, ingress := range []string{"rekor-server", "ctlog", "fulcio-server", "tsa-server"} {
		ingress := &v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ingress,
				Namespace: "testNamespace",
			},
			Spec: v1.IngressSpec{
				Rules: []v1.IngressRule{
					{
						Host: fmt.Sprintf("%s.external.com", ingress),
					},
				},
			},
		}
		err := c.Create(ctx, ingress)
		g.Expect(err).ToNot(HaveOccurred())
	}
	instance := &v1alpha1.Tuf{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "testNamespace",
		},
		Spec: v1alpha1.TufSpec{
			Keys: []v1alpha1.TufKey{
				{
					Name: "rekor.pub",
				},
				{
					Name: "ctfe.pub",
				},
				{
					Name: "fulcio_v1.crt.pem",
				},
				{
					Name: "tsa.certchain.pem",
				},
			},
			SigningConfigURLMode: v1alpha1.SigningConfigURLExternal,
		},
	}
	err := ResolveServiceAddress(ctx, c, instance)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(instance.Spec.Ctlog.Address).To(Equal("http://ctlog.testNamespace.svc"), "ctlog is never exposed externally, so we always use internal mode")

	g.Expect(instance.Spec.Rekor.Address).To(Equal("http://rekor-server.external.com"))
	g.Expect(instance.Spec.Fulcio.Address).To(Equal("http://fulcio-server.external.com"))
	// tsa should have the timestamp path appended
	g.Expect(instance.Spec.Tsa.Address).To(Equal("http://tsa-server.external.com" + tsa.TimestampPath))
}
