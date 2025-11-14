package olm

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExtensionSource interface {
	client.Object
	IsReady(context.Context, client.Client) bool
	UpdateSourceImage(string)
	Unwrap() client.Object
}

type Extension interface {
	client.Object
	IsReady(context.Context, client.Client) bool
	GetVersion(context.Context, client.Client) string
	Unwrap() client.Object
}
