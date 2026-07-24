package actions

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsurePKCS11Config_CanHandle(t *testing.T) {
	tests := []struct {
		name     string
		instance *rhtasv1.Fulcio
		expected bool
	}{
		{
			name: "file CA mode - skip",
			instance: &rhtasv1.Fulcio{
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypeFile,
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
					},
				},
			},
			expected: false,
		},
		{
			name: "pkcs11 mode, creating state, no condition - handle",
			instance: &rhtasv1.Fulcio{
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
					},
				},
			},
			expected: true,
		},
		{
			name: "pkcs11 mode, pending state - skip",
			instance: &rhtasv1.Fulcio{
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Pending"},
					},
				},
			},
			expected: false,
		},
		{
			name: "pkcs11 mode, condition set, no drift - skip",
			instance: &rhtasv1.Fulcio{
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
						PKCS11: &rhtasv1.FulcioPKCS11Config{
							CredentialsRef:  rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cred"}, Key: "pin"},
							PKCS11ConfigRef: rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "conf"}, Key: "crypto11.conf"},
						},
					},
				},
				Status: rhtasv1.FulcioStatus{
					PKCS11: &rhtasv1.FulcioPKCS11Status{
						CredentialsRef:  &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cred"}, Key: "pin"},
						PKCS11ConfigRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "conf"}, Key: "crypto11.conf"},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
						{Type: PKCS11Condition, Status: metav1.ConditionTrue, Reason: "Resolved"},
					},
				},
			},
			expected: false,
		},
		{
			name: "pkcs11 mode, condition set, drift detected - handle",
			instance: &rhtasv1.Fulcio{
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
						PKCS11: &rhtasv1.FulcioPKCS11Config{
							CredentialsRef:  rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "new-cred"}, Key: "pin"},
							PKCS11ConfigRef: rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "conf"}, Key: "crypto11.conf"},
						},
					},
				},
				Status: rhtasv1.FulcioStatus{
					PKCS11: &rhtasv1.FulcioPKCS11Status{
						CredentialsRef:  &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-cred"}, Key: "pin"},
						PKCS11ConfigRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "conf"}, Key: "crypto11.conf"},
					},
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
						{Type: PKCS11Condition, Status: metav1.ConditionTrue, Reason: "Resolved"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewEnsurePKCS11ConfigAction()
			result := a.CanHandle(context.Background(), tt.instance)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEnsurePKCS11Config_Handle(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = rhtasv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name           string
		secrets        []corev1.Secret
		instance       *rhtasv1.Fulcio
		expectError    bool
		expectResolved bool
	}{
		{
			name: "valid refs - resolves successfully",
			secrets: []corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: "hsm-cred", Namespace: "test"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "hsm-config", Namespace: "test"}},
			},
			instance: &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test-fulcio", Namespace: "test"},
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
						PKCS11: &rhtasv1.FulcioPKCS11Config{
							CredentialsRef:  rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-cred"}, Key: "pin"},
							PKCS11ConfigRef: rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-config"}, Key: "crypto11.conf"},
						},
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
					},
				},
			},
			expectResolved: true,
		},
		{
			name:    "missing credentials secret - error",
			secrets: []corev1.Secret{},
			instance: &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test-fulcio", Namespace: "test"},
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
						PKCS11: &rhtasv1.FulcioPKCS11Config{
							CredentialsRef:  rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "missing"}, Key: "pin"},
							PKCS11ConfigRef: rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "conf"}, Key: "crypto11.conf"},
						},
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "nil pkcs11 config - terminal error",
			instance: &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test-fulcio", Namespace: "test"},
				Spec: rhtasv1.FulcioSpec{
					Certificate: rhtasv1.FulcioCert{
						CAType: rhtasv1.CATypePKCS11,
					},
				},
				Status: rhtasv1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: "Creating"},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := make([]runtime.Object, 0)
			for i := range tt.secrets {
				objs = append(objs, &tt.secrets[i])
			}
			objs = append(objs, tt.instance)

			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).WithStatusSubresource(tt.instance).Build()

			a := NewEnsurePKCS11ConfigAction()
			a.InjectClient(client)
			a.InjectLogger(logr.Discard())

			result := a.Handle(context.Background(), tt.instance)

			if tt.expectError {
				g.Expect(action.IsError(result)).To(BeTrue())
			} else if tt.expectResolved {
				cond := meta.FindStatusCondition(tt.instance.Status.Conditions, PKCS11Condition)
				g.Expect(cond).NotTo(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(cond.Reason).To(Equal("Resolved"))
				g.Expect(tt.instance.Status.PKCS11).NotTo(BeNil())
				g.Expect(tt.instance.Status.PKCS11.CredentialsRef).NotTo(BeNil())
				g.Expect(tt.instance.Status.PKCS11.PKCS11ConfigRef).NotTo(BeNil())
			}
		})
	}
}
