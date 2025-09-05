package tls

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/apis"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeObjectWithTlsClient struct {
	client.Object
	apis.TlsClient
}

type fakeTlsClient struct {
	lor v1alpha1.LocalObjectReference
}

func (f fakeTlsClient) GetTrustedCA() *v1alpha1.LocalObjectReference {
	return &f.lor
}

func TestCAPath(t *testing.T) {
	tests := []struct {
		name     string
		objects  []client.Object
		instance objectWithTlsClient
		result   string
		err      bool
	}{
		{
			"trustedCA is defined",
			[]client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: "ca-configmap", Namespace: "tas"},
					Data:       map[string]string{"ca.pem": "PEM data here"},
				},
			},
			fakeObjectWithTlsClient{
				Object: &v1.Pod{
					ObjectMeta: v2.ObjectMeta{
						Namespace: "tas",
					},
				},
				TlsClient: fakeTlsClient{
					lor: v1alpha1.LocalObjectReference{
						Name: "ca-configmap",
					},
				},
			},
			"/var/run/configs/tas/ca-trust/ca.pem",
			false,
		},
		{
			"trustedCA is defined but configmap not found",
			[]client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: "ca-configmap", Namespace: "tas"},
					Data:       map[string]string{"ca.pem": "PEM data here"},
				},
			},
			fakeObjectWithTlsClient{
				Object: &v1.Pod{
					ObjectMeta: v2.ObjectMeta{
						Namespace: "wrong-namespace",
					},
				},
				TlsClient: fakeTlsClient{
					lor: v1alpha1.LocalObjectReference{
						Name: "wrong-name",
					},
				},
			},
			"",
			true,
		},
		{
			"trustedCA is defined but configmap has multiple keys",
			[]client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: "ca-configmap", Namespace: "tas"},
					Data:       map[string]string{"ca.pem": "PEM data here", "not": "supposed to be here"},
				},
			},
			fakeObjectWithTlsClient{
				Object: &v1.Pod{
					ObjectMeta: v2.ObjectMeta{
						Namespace: "tas",
					},
				},
				TlsClient: fakeTlsClient{
					lor: v1alpha1.LocalObjectReference{
						Name: "ca-configmap",
					},
				},
			},
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := gomega.NewWithT(t)

			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			result, err := CAPath(ctx, c, tt.instance)
			if tt.err {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
			g.Expect(result).To(gomega.Equal(tt.result))
		})
	}
}
