package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const testRootPEM = `-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIRAIoLg5mVhoyOGw9VDqL5dSMwCgYIKoZIzj0EAwIwEjEQ
MA4GA1UEChMHUkhUQVMwHhcNMjYwMTAxMDAwMDAwWhcNMjcwMTAxMDAwMDAwWjAS
MRAwDgYDVQQKEwdSSFRBUzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABPxDEU0E
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123
o0IwQDAOBgNVHQ8BAf8EBAMCAQYwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQU
0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopwCgYIKoZIzj0E
AwIDSAAwRQIhAJ6789abcdef0123456789abcdef0123456789abcdef01234567AB
-----END CERTIFICATE-----`

func TestExtractHSMRoot_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		instance  *rhtasv1alpha1.Fulcio
		objects   []client.Object
		canHandle bool
	}{
		{
			name: "file CA type - skip",
			instance: &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypeFile},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			},
			canHandle: false,
		},
		{
			name: "pkcs11 in Initialize state, no existing secret",
			instance: &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			},
			canHandle: true,
		},
		{
			name: "pkcs11 but not in Initialize state",
			instance: &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Creating.String()},
					},
				},
			},
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			builder := testAction.FakeClientBuilder().
				WithObjects(tt.instance)
			for _, obj := range tt.objects {
				builder = builder.WithObjects(obj)
			}
			c := builder.Build()
			a := testAction.PrepareAction(c, NewExtractHSMRootAction())
			g.Expect(a.CanHandle(context.TODO(), tt.instance)).To(Equal(tt.canHandle))
		})
	}
}

func TestExtractHSMRoot_Handle(t *testing.T) {
	ctx := context.TODO()

	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.Fulcio, client.WithWatch)
	}
	tests := []struct {
		name       string
		httpStatus int
		httpBody   string
		want       want
	}{
		{
			name:       "successful root cert extraction",
			httpStatus: http.StatusOK,
			httpBody:   testRootPEM,
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, cli client.WithWatch) {
					g.Expect(instance.Status.Certificate.CARef).NotTo(BeNil())
					g.Expect(instance.Status.Certificate.CARef.Key).To(Equal("cert"))

					secret, err := kubernetes.FindSecret(ctx, cli, "default", FulcioCALabel)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(secret.Name).To(Equal(instance.Status.Certificate.CARef.Name))
				},
			},
		},
		{
			name:       "fulcio returns 503 — requeue",
			httpStatus: http.StatusServiceUnavailable,
			httpBody:   "not ready",
			want: want{
				result: testAction.Requeue(),
				verify: func(g Gomega, instance *rhtasv1alpha1.Fulcio, _ client.WithWatch) {
					g.Expect(instance.Status.Certificate.CARef).To(BeNil())
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.httpStatus)
				_, _ = w.Write([]byte(tt.httpBody))
			}))
			defer srv.Close()

			instance := &rhtasv1alpha1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-fulcio",
					Namespace: "default",
				},
				Spec: rhtasv1alpha1.FulcioSpec{
					Certificate: rhtasv1alpha1.FulcioCert{CAType: rhtasv1alpha1.CATypePKCS11},
				},
				Status: rhtasv1alpha1.FulcioStatus{
					Certificate: &rhtasv1alpha1.FulcioCert{
						CAType: rhtasv1alpha1.CATypePKCS11,
					},
					Url: srv.URL,
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewExtractHSMRootAction())
			result := a.Handle(ctx, instance)
			g.Expect(result).To(Equal(tt.want.result))

			found := &rhtasv1alpha1.Fulcio{}
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
			tt.want.verify(g, found, c)
		})
	}
}
