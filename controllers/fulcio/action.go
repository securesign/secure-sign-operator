package fulcio

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/api/v1alpha1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Action interface {
	InjectClient(client client.Client)
	InjectRecorder(recorder record.EventRecorder)
	InjectLogger(logger logr.Logger)

	// a user friendly name for the action
	Name() string

	// returns true if the action can handle the integration
	CanHandle(*v1alpha1.Fulcio) bool

	// executes the handling function
	Handle(context.Context, *v1alpha1.Fulcio) (*v1alpha1.Fulcio, error)
}

type BaseAction struct {
	Client   client.Client
	Recorder record.EventRecorder
}

func (action *BaseAction) InjectClient(client client.Client) {
	action.Client = client
}

func (action *BaseAction) InjectRecorder(recorder record.EventRecorder) {
	action.Recorder = recorder
}
