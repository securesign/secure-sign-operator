package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestExistsSecret(t *testing.T) {
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
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				WithInterceptorFuncs(tt.intercept).
				Build()

			exists, err := ExistsSecret(context.TODO(), c, "default", "my-secret")
			g.Expect(exists).To(gomega.Equal(tt.exists))
			if tt.wantErr {
				g.Expect(err).To(gomega.HaveOccurred())
			} else {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			}
		})
	}
}

func TestEnsureSecret(t *testing.T) {
	data := map[string][]byte{"test": []byte("data")}
	tests := []struct {
		name      string
		objects   []client.Object
		immutable bool
		result    controllerutil.OperationResult
		wantErr   error
	}{
		{
			name:      "create new mutable object",
			objects:   []client.Object{},
			result:    controllerutil.OperationResultCreated,
			immutable: false,
		},
		{
			name:      "create new immutable object",
			objects:   []client.Object{},
			result:    controllerutil.OperationResultCreated,
			immutable: true,
		},
		{
			name: "update mutable existing object data",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string][]byte{"foo": []byte("bar")},
				},
			},
			immutable: false,
			result:    controllerutil.OperationResultUpdated,
		},
		{
			name: "make existing object immutable",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string][]byte{"test": []byte("data")},
				},
			},
			result:    controllerutil.OperationResultUpdated,
			immutable: true,
		},
		{
			name: "update immutable existing object data",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string][]byte{"foo": []byte("bar")},
					Immutable:  utils.Pointer(true),
				},
			},
			immutable: true,
			result:    controllerutil.OperationResultNone,
			wantErr:   ErrImmutableSecretDataMismatch,
		},
		{
			name: "update immutable object mutability",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string][]byte{"test": []byte("data")},
					Immutable:  utils.Pointer(true),
				},
			},
			immutable: false,
			result:    controllerutil.OperationResultNone,
			wantErr:   ErrImmutableSecretMutability,
		},
		{
			name: "existing object with expected values",
			objects: []client.Object{
				&v1.Secret{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string][]byte{"test": []byte("data")},
					Immutable:  utils.Pointer(true),
				},
			},
			immutable: true,
			result:    controllerutil.OperationResultNone,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			result, err := CreateOrUpdate(ctx, c,
				&v1.Secret{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureSecretData(tt.immutable, data))

			g.Expect(result).To(gomega.Equal(tt.result))

			if tt.wantErr == nil {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(err).To(gomega.HaveOccurred())
				g.Expect(errors.Is(err, tt.wantErr)).To(gomega.BeTrue(), "expected error wrapping %v, got %v", tt.wantErr, err)
				return
			}

			existing := &v1.Secret{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			g.Expect(utils.OptionalBool(existing.Immutable)).To(gomega.Equal(tt.immutable))
			g.Expect(existing.Data).To(gomega.Equal(data))
		})
	}
}
