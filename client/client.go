package client

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controller "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Client is an abstraction for a k8s client.
type Client interface {
	controller.Client
	kubernetes.Interface
	GetScheme() *runtime.Scheme
	GetConfig() *rest.Config
}

type defaultClient struct {
	controller.Client
	kubernetes.Interface
	scheme *runtime.Scheme
	config *rest.Config
}

func (c *defaultClient) GetScheme() *runtime.Scheme {
	return c.scheme
}

func (c *defaultClient) GetConfig() *rest.Config {
	return c.config
}

// NewClient creates a new k8s client that can be used from outside or in the cluster.
func NewClientWithScheme(scheme *runtime.Scheme) (Client, error) {
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	var clientset kubernetes.Interface
	if clientset, err = kubernetes.NewForConfig(cfg); err != nil {
		return nil, err
	}

	// Create a new client to avoid using cache (enabled by default on operator-sdk client)
	clientOptions := controller.Options{
		Scheme: scheme,
	}
	dynClient, err := controller.New(cfg, clientOptions)
	if err != nil {
		return nil, err
	}

	return &defaultClient{
		Client:    dynClient,
		Interface: clientset,
		scheme:    clientOptions.Scheme,
		config:    cfg,
	}, nil
}

func NewClient() (Client, error) {
	return NewClientWithScheme(clientscheme.Scheme)
}
