package action

import (
	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func FakeClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	cl := fake.NewClientBuilder().WithScheme(scheme)
	return cl
}

func PrepareAction[T apis.ConditionsAwareObject](c client.Client, a action.Action[T]) action.Action[T] {
	a.InjectClient(c)
	a.InjectLogger(logr.Logger{})
	a.InjectRecorder(record.NewFakeRecorder(10))
	return a
}
