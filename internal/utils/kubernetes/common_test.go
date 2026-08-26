package kubernetes

import (
	"context"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mutateErr error
		intercept interceptor.Funcs
		wantErr   bool
	}{
		{
			name: "applies mutate fns and creates, without probing for an existing object",
			intercept: interceptor.Funcs{
				Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
					return fmt.Errorf("Get should never be called by Create")
				},
			},
		},
		{
			name:      "mutate fn error is returned and Create is not called",
			mutateErr: fmt.Errorf("mutate failed"),
			intercept: interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
					return fmt.Errorf("Create should not be called when a mutate fn fails")
				},
			},
			wantErr: true,
		},
		{
			name: "client Create error is propagated",
			intercept: interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
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
				WithInterceptorFuncs(tt.intercept).
				Build()

			obj := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-secret-",
					Namespace:    "test-namespace",
				},
			}

			err := Create(t.Context(), c, obj,
				func(s *corev1.Secret) error {
					if tt.mutateErr != nil {
						return tt.mutateErr
					}
					s.Labels = map[string]string{"test-key": "test-value"}
					return nil
				},
			)

			if tt.wantErr {
				g.Expect(err).To(gomega.HaveOccurred())
				return
			}
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(obj.Name).ToNot(gomega.BeEmpty())
			g.Expect(obj.Labels).To(gomega.Equal(map[string]string{"test-key": "test-value"}))
		})
	}
}
