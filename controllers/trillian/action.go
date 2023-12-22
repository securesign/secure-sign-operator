package trillian

import (
	"context"

	"github.com/securesign/operator/api/v1alpha1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Action interface {
	InjectClient(client client.Client)
	InjectRecorder(recorder record.EventRecorder)

	// a user friendly name for the action
	Name() string

	// returns true if the action can handle the integration
	CanHandle(trillian *v1alpha1.Trillian) bool

	// executes the handling function
	Handle(ctx context.Context, trillian *v1alpha1.Trillian) (*v1alpha1.Trillian, error)
}
