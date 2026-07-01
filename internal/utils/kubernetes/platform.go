package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const minAPIServiceTimeout = 15 * time.Second

//+kubebuilder:rbac:groups=apiregistration.k8s.io,resources=apiservices,verbs=get;list;watch

// DetectOpenShiftPlatform detects whether the operator is running on OpenShift.
// It checks for API services with the specified OpenShift API service name.
// Transient connectivity errors (connection refused, EOF, 503) are retried with
// exponential backoff, with a minimum timeout of 15 seconds.
func DetectOpenShiftPlatform(log logr.Logger, apiServiceName string, apiServiceTimeout time.Duration) (bool, error) {
	if apiServiceName == "" {
		return false, nil
	}
	if apiServiceTimeout <= minAPIServiceTimeout {
		log.Info("APIServiceTimeout too low, defaulting to minimum timeout", "apiServiceTimeout", apiServiceTimeout, "minAPIServiceTimeout", minAPIServiceTimeout)
		apiServiceTimeout = minAPIServiceTimeout
	}
	log.Info("APIServiceTimeout", "apiServiceTimeout", apiServiceTimeout)
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

	ctx, cancel := context.WithTimeout(context.Background(), apiServiceTimeout)
	defer cancel()

	backoff := wait.Backoff{
		Duration: 2 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
		Steps:    5,
	}

	return detectOpenShiftWithRetry(ctx, log, cl, apiServiceName, backoff)
}

func detectOpenShiftWithRetry(ctx context.Context, log logr.Logger, cl client.Client, apiServiceName string, backoff wait.Backoff) (bool, error) {
	var found bool
	var lastErr error
	retryErr := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		apiServiceList := &apiregistrationv1.APIServiceList{}
		if listErr := cl.List(ctx, apiServiceList); listErr != nil {
			if isTransientError(listErr) {
				log.Info("Transient error during OpenShift platform detection, retrying", "error", listErr)
				lastErr = listErr
				return false, nil
			}
			return false, listErr
		}
		for _, apiService := range apiServiceList.Items {
			if service := apiService.Spec.Service; service != nil {
				// The service will be default/openshift-apiserver or openshift-apiserver/api
				if apiServiceName == service.Namespace || apiServiceName == service.Name {
					log.Info("Discovered APIService matching API service name", "namespace", service.Namespace, "name", service.Name)
					found = true
					return true, nil
				}
			}
		}
		return true, nil
	})
	if retryErr != nil {
		if wait.Interrupted(retryErr) {
			if lastErr != nil {
				return false, fmt.Errorf("timed out waiting for API server during OpenShift platform detection: %w", lastErr)
			}
			return false, errors.New("timed out waiting for API server during OpenShift platform detection")
		}
		return false, retryErr
	}
	return found, nil
}

func isTransientError(err error) bool {
	if apierrors.IsServiceUnavailable(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return isTransientError(urlErr.Err)
	}
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, io.EOF)
}
