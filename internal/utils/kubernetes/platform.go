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
	err := RetryOnTransient(ctx, log, backoff, "OpenShift platform detection", func(ctx context.Context) error {
		apiServiceList := &apiregistrationv1.APIServiceList{}
		if err := cl.List(ctx, apiServiceList); err != nil {
			return err
		}
		for _, apiService := range apiServiceList.Items {
			if service := apiService.Spec.Service; service != nil {
				// The service will be default/openshift-apiserver or openshift-apiserver/api
				if apiServiceName == service.Namespace || apiServiceName == service.Name {
					log.Info("Discovered APIService matching API service name", "namespace", service.Namespace, "name", service.Name)
					found = true
					return nil
				}
			}
		}
		return nil
	})
	return found, err
}

// RetryOnTransient runs op with exponential backoff, retrying only on transient
// API/network errors (see isTransientError); non-transient errors are returned
// immediately. description is used in retry logs and in the timeout error
// returned when the context is cancelled or the backoff is exhausted.
func RetryOnTransient(ctx context.Context, log logr.Logger, backoff wait.Backoff, description string, op func(ctx context.Context) error) error {
	var lastErr error
	retryErr := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		if err := op(ctx); err != nil {
			if isTransientError(err) {
				log.Info("Transient error during "+description+", retrying", "error", err)
				lastErr = err
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if retryErr != nil {
		if wait.Interrupted(retryErr) {
			if lastErr != nil {
				return fmt.Errorf("timed out during %s: %w", description, lastErr)
			}
			return fmt.Errorf("timed out during %s", description)
		}
		return retryErr
	}
	return nil
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
