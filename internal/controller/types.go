package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller interface {
	SetupWithManager(ctrl.Manager) error
}

type Constructor func(client.Client, *runtime.Scheme, record.EventRecorder) Controller
