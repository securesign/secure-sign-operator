package action

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func FakeClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rhtasv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme).WithReturnManagedFields()
	return cl
}

const TestFieldManager = "securesign-operator"

// legacyFieldManager is the field manager the pre-SSA reconciler
// (controllerutil.CreateOrUpdate, an Update operation) left on objects already
// running on clusters; it mirrors kubernetes.LegacyFieldManager. It cannot be
// imported from that package here because the kubernetes package's own tests
// import this one, which would form a test import cycle.
const legacyFieldManager = "manager"

// FakeClientWithObjects builds a fake client seeded with the given objects as an
// upgraded, pre-SSA cluster holds them: each object is created under the legacy
// "manager" field owner (an Update-operation ownership), exactly what the old
// controllerutil.CreateOrUpdate reconciler left behind. This lets
// kubernetes.CreateOrUpdate exercise its managed-fields migration and
// server-side-apply pruning the way it does against a real cluster.
//
// Unlike a plain WithObjects seed (which records no field ownership) this makes
// the operator the sole owner of the fields it manages after migration, so
// fields it stops declaring are pruned.
func FakeClientWithObjects(objects ...client.Object) client.WithWatch {
	builder := FakeClientBuilder()

	var statusObjects []client.Object
	for _, obj := range objects {
		if hasStatusField(obj) {
			statusObjects = append(statusObjects, obj)
		}
	}
	if len(statusObjects) > 0 {
		builder = builder.WithStatusSubresource(statusObjects...)
	}
	cli := builder.Build()

	ctx := context.Background()
	for _, obj := range objects {
		if err := cli.Create(ctx, obj, client.FieldOwner(legacyFieldManager)); err != nil {
			panic(fmt.Sprintf("FakeClientWithObjects: Create: %v", err))
		}
	}
	return cli
}

func hasStatusField(obj client.Object) bool {
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	_, found := t.FieldByName("Status")
	return found
}

func PrepareAction[T apis.ConditionsAwareObject](c client.Client, a action.Action[T]) action.Action[T] {
	a.InjectClient(c)
	a.InjectLogger(logr.Logger{})
	a.InjectRecorder(events.NewFakeRecorder(10))
	return a
}
