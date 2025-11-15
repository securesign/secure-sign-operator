package webhook_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/securesign/operator/api/v1alpha1"
	webhook "github.com/securesign/operator/internal/webhook"
)

func GenerateSecuresignObj(namespace string, labels map[string]string) *v1alpha1.Securesign {
	return &v1alpha1.Securesign{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rhtas.redhat.com/v1alpha1",
			Kind:       "Securesign",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: namespace,
			Labels:    labels,
		},
	}
}

func TestSecureSignValidator(t *testing.T) {
	mockNsReserved := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			Labels: map[string]string{
				"openshift.io/run-level": "0",
			},
		},
	}
	mockNsAllowed := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-valid-ns",
		},
	}

	c := fake.NewClientBuilder().WithObjects(mockNsReserved, mockNsAllowed).Build()

	validator := webhook.SecureSignValidator{
		Client: c,
	}

	tests := []struct {
		name      string
		obj       runtime.Object
		expectErr bool
	}{
		{
			name:      "Case 1: Allowed Dynamic Namespace",
			obj:       GenerateSecuresignObj("test-valid-ns", nil),
			expectErr: false,
		},
		{
			name:      "Case 2: Denied Default Namespace",
			obj:       GenerateSecuresignObj("default", nil),
			expectErr: true,
		},
		{
			name:      "Case 3: Denied Reserved Openshift Namespace",
			obj:       GenerateSecuresignObj("kube-system", nil),
			expectErr: true,
		},
		{
			name:      "Case 4: Wrong Resource Type (Denial)",
			obj:       &unstructured.Unstructured{},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			_, createErr := validator.ValidateCreate(ctx, tc.obj)
			if tc.expectErr {
				require.Error(t, createErr, "ValidateCreate expected an error but got nil.")
			} else {
				require.NoError(t, createErr, "ValidateCreate returned an unexpected error.")
			}

			_, updateErr := validator.ValidateUpdate(ctx, tc.obj, tc.obj)
			if tc.expectErr {
				require.Error(t, updateErr, "ValidateUpdate expected an error but got nil.")
			} else {
				require.NoError(t, updateErr, "ValidateUpdate returned an unexpected error.")
			}

			_, deleteErr := validator.ValidateDelete(ctx, tc.obj)
			require.NoError(t, deleteErr, "ValidateDelete unexpectedly returned an error.")
		})
	}
}
