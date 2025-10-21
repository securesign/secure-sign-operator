package webhooks

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (v *SecureSignValidator) validateNamespacePolicy(ctx context.Context, operandCR *rhtasv1alpha1.Securesign) (admission.Warnings, error) {
	reqLog := logf.FromContext(ctx)
	targetNamespace := operandCR.GetNamespace()

	if targetNamespace == "default" {
		reqLog.Info("Validation failed: Deployment blocked in 'default' namespace.")
		return nil, fmt.Errorf("installation into the 'default' namespace is prohibited by RHTAS policy")
	}

	ns := &corev1.Namespace{}

	if err := v.Client.Get(ctx, types.NamespacedName{Name: targetNamespace}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		reqLog.Error(err, "Failed to retrieve target namespace object for validation.")
		return nil, fmt.Errorf("failed to retrieve target namespace %s: %w", targetNamespace, err)
	}

	runLevel, found := ns.Labels["openshift.io/run-level"]
	if found && reservedRunLevels[runLevel] {
		reqLog.Info("Validation failed: Deployment blocked in reserved namespace.",
			"namespace", targetNamespace, "run-level", runLevel)
		return nil, fmt.Errorf("installation into reserved OpenShift namespace '%s' (run-level %s) is prohibited by RHTAS policy", targetNamespace, runLevel)
	}

	return nil, nil
}

func (v *SecureSignValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	operandCR, ok := obj.(*rhtasv1alpha1.Securesign)
	if !ok {
		return nil, fmt.Errorf("expected SecureSign CR but got %T", obj)
	}
	return v.validateNamespacePolicy(ctx, operandCR)
}

func (v *SecureSignValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	operandCR, ok := newObj.(*rhtasv1alpha1.Securesign)
	if !ok {
		return nil, fmt.Errorf("expected SecureSign CR but got %T", newObj)
	}
	return v.validateNamespacePolicy(ctx, operandCR)
}

func (v *SecureSignValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// Allow all delete operations
	return nil, nil
}
