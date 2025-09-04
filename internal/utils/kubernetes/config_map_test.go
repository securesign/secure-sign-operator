package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureImmutableConfigMap(t *testing.T) {
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
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string]string{"foo": "bar"},
				},
			},
			immutable: false,
			result:    controllerutil.OperationResultUpdated,
		},
		{
			name: "make existing object immutable",
			objects: []client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string]string{"test": "data"},
				},
			},
			result:    controllerutil.OperationResultUpdated,
			immutable: true,
		},
		{
			name: "update immutable existing object data",
			objects: []client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string]string{"foo": "bar"},
					Immutable:  utils.Pointer(true),
				},
			},
			immutable: true,
			result:    controllerutil.OperationResultNone,
			errorMsg:  "can't update immutable ConfigMap data",
		},
		{
			name: "update immutable object mutability",
			objects: []client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string]string{"test": "data"},
					Immutable:  utils.Pointer(true),
				},
			},
			immutable: false,
			result:    controllerutil.OperationResultNone,
			errorMsg:  "can't make update ConfigMap mutability",
		},
		{
			name: "existing object with expected values",
			objects: []client.Object{
				&v1.ConfigMap{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Data:       map[string]string{"test": "data"},
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
				&v1.ConfigMap{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureConfigMapData(tt.immutable, map[string]string{"test": "data"}))

			g.Expect(result).To(gomega.Equal(tt.result))

			if tt.errorMsg == "" {
				g.Expect(err).ToNot(gomega.HaveOccurred())
			} else {
				g.Expect(err.Error()).To(gomega.Equal(tt.errorMsg))
				return
			}

			existing := &v1.ConfigMap{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			g.Expect(existing.Data).To(gomega.Equal(existing.Data))
			g.Expect(utils.OptionalBool(existing.Immutable)).To(gomega.Equal(tt.immutable))
		})
	}
}
