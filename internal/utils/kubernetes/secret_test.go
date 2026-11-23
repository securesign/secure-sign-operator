package kubernetes

import (
	"context"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestExistsSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		objects   []client.Object
		intercept interceptor.Funcs
		exists    bool
		wantErr   bool
	}{
		{
			name: "secret exists",
			objects: []client.Object{
				&v1.Secret{ObjectMeta: v2.ObjectMeta{Name: "my-secret", Namespace: "default"}},
			},
			exists: true,
		},
		{
			name:   "secret not found",
			exists: false,
		},
		{
			name: "transient API error",
			intercept: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("api server unavailable")
				},
			},
			exists:  false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				WithInterceptorFuncs(tt.intercept).
				Build()

			exists, err := ExistsSecret(t.Context(), c, "default", "my-secret")
			g.Expect(exists).To(gomega.Equal(tt.exists))
			if tt.wantErr {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestGetSecretMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		objects   []client.Object
		intercept interceptor.Funcs
		wantAnn   map[string]string
		wantErr   bool
	}{
		{
			name: "secret exists",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{
						Name:        "my-secret",
						Namespace:   "default",
						Annotations: map[string]string{"foo": "bar"},
					},
					Data: map[string][]byte{"key": []byte("should-not-be-needed")},
				},
			},
			wantAnn: map[string]string{"foo": "bar"},
		},
		{
			name:    "secret not found",
			wantErr: true,
		},
		{
			name: "transient API error",
			intercept: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("api server unavailable")
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				WithInterceptorFuncs(tt.intercept).
				Build()

			meta, err := GetSecretMetadata(t.Context(), c, "default", "my-secret")
			if tt.wantErr {
				g.Expect(err).To(gomega.HaveOccurred())
				return
			}
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(meta.GetAnnotations()).To(gomega.Equal(tt.wantAnn))
		})
	}
}
