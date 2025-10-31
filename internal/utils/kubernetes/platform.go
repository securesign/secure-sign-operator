package kubernetes

import (
	"context"
	"fmt"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	OperatorConfigName      = "rhtas-operator-config"
	PlatformOpenShift       = "openshift"
	PlatformKubernetes      = "kubernetes"
	OpenShiftAPIGroupSuffix = ".openshift.io"
)

// DetectPlatform attempts to determine if the cluster is OpenShift or vanilla Kubernetes
// by checking for OpenShift-specific API groups (*.openshift.io)
func DetectPlatform(ctx context.Context) (string, error) {
	var cfg *rest.Config
	var err error

	cfg, err = config.GetConfig()
	if err != nil {
		return PlatformKubernetes, nil
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return PlatformKubernetes, nil
	}

	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		return PlatformKubernetes, nil
	}

	for _, group := range apiGroupList.Groups {
		if strings.HasSuffix(group.Name, OpenShiftAPIGroupSuffix) {
			return PlatformOpenShift, nil
		}
	}

	return PlatformKubernetes, nil
}

// createOperatorConfig creates a new TASOperatorConfig resource with the given platform and detection method
func createOperatorConfig(ctx context.Context, cl client.Client, platform, detectionMethod string) (*rhtasv1alpha1.TASOperatorConfig, error) {
	config := &rhtasv1alpha1.TASOperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: OperatorConfigName,
		},
		Spec: rhtasv1alpha1.TASOperatorConfigSpec{
			Platform: platform,
		},
	}

	if err := cl.Create(ctx, config); err != nil {
		// If multiple operator pods start simultaneously, only one will
		// successfully create the resource. Others will get AlreadyExists error.
		// In that case, fetch the existing resource instead of failing.
		// Note: the operator config status may be nil if we detect the creation
		// before the status has been updated
		if errors.IsAlreadyExists(err) {
			existingConfig := &rhtasv1alpha1.TASOperatorConfig{}
			if getErr := cl.Get(ctx, types.NamespacedName{Name: OperatorConfigName}, existingConfig); getErr == nil {
				return existingConfig, nil
			}
		}
		return nil, fmt.Errorf("error creating operator config: %w", err)
	}

	config.Status.DetectionMethod = detectionMethod
	config.Status.DetectionTimestamp = metav1.Now()
	if err := cl.Status().Update(ctx, config); err != nil {
		return nil, fmt.Errorf("error updating operator config status: %w", err)
	}

	return config, nil
}

// GetOrAutoDetectConfig retrieves the existing TASOperatorConfig or creates a new one
// with auto-detected platform settings
func GetOrAutoDetectConfig(ctx context.Context, cl client.Client) (*rhtasv1alpha1.TASOperatorConfig, error) {
	config := &rhtasv1alpha1.TASOperatorConfig{}
	err := cl.Get(ctx, types.NamespacedName{Name: OperatorConfigName}, config)
	if err == nil {
		return config, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error retrieving operator config: %w", err)
	}

	platform, err := DetectPlatform(ctx)
	if err != nil {
		return nil, fmt.Errorf("error detecting platform for new config: %w", err)
	}

	return createOperatorConfig(ctx, cl, platform, "auto-detected")
}

// ResolvePlatform retrieves the platform from the existing TASOperatorConfig,
// or creates it with the requested platform if it doesn't exist.
// The TASOperatorConfig is the source of truth - if it exists, its platform value takes precedence
// and will override the caller's requested platform.
func ResolvePlatform(ctx context.Context, cl client.Client, requestedPlatform string, detectionMethod string) (string, error) {
	config := &rhtasv1alpha1.TASOperatorConfig{}
	err := cl.Get(ctx, types.NamespacedName{Name: OperatorConfigName}, config)
	if err != nil {
		if errors.IsNotFound(err) {
			newConfig, err := createOperatorConfig(ctx, cl, requestedPlatform, detectionMethod)
			if err != nil {
				return "", err
			}
			return newConfig.Spec.Platform, nil
		}
		return "", fmt.Errorf("error retrieving operator config: %w", err)
	}

	return config.Spec.Platform, nil
}
