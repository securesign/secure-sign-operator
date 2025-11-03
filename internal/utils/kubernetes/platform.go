package kubernetes

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

//+kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=get;list;watch

// DetectOpenShiftPlatform detects whether the operator is running on OpenShift.
// It checks for API services with the specified OpenShift API service name.
func DetectOpenShiftPlatform(log logr.Logger, apiServiceName string) (bool, error) {
	if apiServiceName == "" {
		return false, nil
	}
	cfg, err := config.GetConfig()
	if err != nil {
		return false, err
	}
	scheme := runtime.NewScheme()
	err = apiregistrationv1.SchemeBuilder.AddToScheme(scheme)
	if err != nil {
		return false, err
	}
	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return false, err
	}

	apiServiceList := &apiregistrationv1.APIServiceList{}
	if err := cl.List(context.Background(), apiServiceList); err != nil {
		return false, err
	}

	for _, apiService := range apiServiceList.Items {
		if service := apiService.Spec.Service; service != nil {
			// The service will be default/openshift-apiserver or openshift-apiserver/api
			if apiServiceName == service.Namespace || apiServiceName == service.Name {
				log.Info("Discovered APIService matching API service name", "namespace", service.Namespace, "name", service.Name)
				return true, nil
			}
		}
	}

	return false, nil
}
