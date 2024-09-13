package utils

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func UseTLS(ctx context.Context, client client.Client, instance *rhtasv1alpha1.Fulcio) (bool, error) {

	if instance == nil {
		return false, nil
	}

	service, err := kubernetes.GetService(client, instance.Namespace, "ctlog")
	if err != nil {
		return false, fmt.Errorf("failed to get ctlog service: %w", err)
	}

	for _, port := range service.Spec.Ports {
		if port.Name == "https" || port.Port == 443 {
			return true, nil
		}
	}
	return kubernetes.IsOpenShift(), nil
}

func CAPath(ctx context.Context, cli client.Client, instance *rhtasv1alpha1.Fulcio) (string, error) {
	if instance.Spec.TrustedCA != nil {
		cfgTrust, err := kubernetes.GetConfigMap(ctx, cli, instance.Namespace, instance.Spec.TrustedCA.Name)
		if err != nil {
			return "", err
		}
		if len(cfgTrust.Data) != 1 {
			err = fmt.Errorf("%s ConfigMap can contain only 1 record", instance.Spec.TrustedCA.Name)
			return "", err
		}
		for key := range cfgTrust.Data {
			return "/var/run/configs/tas/ca-trust/" + key, nil
		}
	}

	if instance.Spec.TrustedCA == nil && kubernetes.IsOpenShift() {
		return "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", nil
	}

	return "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil
}
