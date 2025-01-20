package trillianUtils

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"golang.org/x/exp/maps"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func UseTLS(instance *rhtasv1alpha1.Trillian) bool {

	if instance == nil {
		return false
	}

	// when DB is managed by operator
	if utils.IsEnabled(instance.Spec.Db.Create) && instance.Status.Db.TLS.CertRef != nil {
		return true
	}

	// external DB
	if !utils.IsEnabled(instance.Spec.Db.Create) && instance.Spec.TrustedCA != nil {
		return true
	}

	return false
}

func CAPath(ctx context.Context, cli client.Client, instance *rhtasv1alpha1.Trillian) (string, error) {
	switch {
	case instance.Spec.TrustedCA != nil:
		cfgTrust, err := kubernetes.GetConfigMap(ctx, cli, instance.Namespace, instance.Spec.TrustedCA.Name)
		if err != nil {
			return "", err
		}
		if len(cfgTrust.Data) != 1 {
			err = fmt.Errorf("%s ConfigMap can contain only 1 record", instance.Spec.TrustedCA.Name)
			return "", err
		}
		return ensure.CATRustMountPath + maps.Keys(cfgTrust.Data)[0], nil
	case kubernetes.IsOpenShift():
		return "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt", nil
	default:
		return "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", nil
	}
}
