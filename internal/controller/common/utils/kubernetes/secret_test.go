package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/utils"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureSecret(t *testing.T) {
	gomega.RegisterTestingT(t)
	tests := []struct {
		name      string
		objects   []client.Object
		immutable bool
		result    controllerutil.OperationResult
		errorMsg  string
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
			errorMsg:  "can't update immutable Secret data",
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
			errorMsg:  "can't make update Secret mutability",
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
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			result, err := CreateOrUpdate(ctx, c,
				&v1.Secret{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureSecretData(tt.immutable, map[string][]byte{"test": []byte("data")}))

			gomega.Expect(result).To(gomega.Equal(tt.result))

			if tt.errorMsg == "" {
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				gomega.Expect(err.Error()).To(gomega.Equal(tt.errorMsg))
				return
			}

			existing := &v1.Secret{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Data).To(gomega.Equal(existing.Data))
			gomega.Expect(utils.OptionalBool(existing.Immutable)).To(gomega.Equal(tt.immutable))
		})
	}
}
