package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestService(t *testing.T) {
	gomega.RegisterTestingT(t)
	tests := []struct {
		name    string
		objects []client.Object
		result  controllerutil.OperationResult
	}{
		{
			"create new object",
			[]client.Object{},
			controllerutil.OperationResultCreated,
		},
		{
			"update existing object",
			[]client.Object{
				&v1.Service{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{Name: "http", Port: 8080},
							{Name: "http", Port: 80},
						},
						Selector: map[string]string{"foo": "bar"},
					},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&v1.Service{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.ServiceSpec{
						Ports: []v1.ServicePort{
							{Name: "http", Port: 80},
						},
						Selector: map[string]string{"testLabel": "testValue"},
					},
				},
			},
			controllerutil.OperationResultNone,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			ports := []v1.ServicePort{{Name: "http", Port: 80}}
			l := map[string]string{"testLabel": "testValue"}

			result, err := CreateOrUpdate(ctx, c,
				&v1.Service{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureServiceSpec(l, ports...))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &v1.Service{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Spec.Ports).To(gomega.Equal(ports))
			gomega.Expect(existing.Spec.Selector).To(gomega.Equal(l))
		})
	}
}
