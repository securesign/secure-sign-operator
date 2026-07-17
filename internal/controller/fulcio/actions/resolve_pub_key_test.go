package actions

import (
	_ "embed"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	httpmock "github.com/securesign/operator/internal/testing/http"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed testdata/signing_cert.pem
var testSigningCertPEM string

//go:embed testdata/root_cert.pem
var testRootCertPEM string

// testSigningCert and testRootCert hold the certs with literal "\n" escapes,
// as they'd appear inside a JSON string body.
var testSigningCert = strings.ReplaceAll(strings.TrimSpace(testSigningCertPEM), "\n", "\\n")
var testRootCert = strings.ReplaceAll(strings.TrimSpace(testRootCertPEM), "\n", "\\n")

var testTrustBundleJSON = `{
	"chains": [{
		"certificates": [
			"` + testSigningCert + `",
			"` + testRootCert + `"
		]
	}]
}`

var expectedRootCert = strings.TrimSpace(testSigningCertPEM) + "\n" + strings.TrimSpace(testRootCertPEM)

func TestFulcioResolvePubKey_CanHandle(t *testing.T) {
	t.Parallel()
	a := NewResolvePubKeyAction()
	t.Run("not ready", func(t *testing.T) {
		t.Parallel()
		instance := &rhtasv1.Fulcio{}
		if a.CanHandle(t.Context(), instance) {
			t.Error("expected false when no condition set")
		}
	})
	t.Run("initialize phase", func(t *testing.T) {
		t.Parallel()
		instance := &rhtasv1.Fulcio{}
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String(),
		})
		if !a.CanHandle(t.Context(), instance) {
			t.Error("expected true in Initialize phase")
		}
	})
}

func TestFulcioResolvePubKey_Handle(t *testing.T) {
	g := NewWithT(t)
	type want struct {
		result          *action.Result
		rootCertificate string
	}
	tests := []struct {
		name            string
		rootCertificate string
		httpStatus      int
		httpBody        string
		want            want
	}{
		{
			name:       "resolve trust bundle from API",
			httpStatus: http.StatusOK,
			httpBody:   testTrustBundleJSON,
			want: want{
				result:          testAction.Continue(),
				rootCertificate: expectedRootCert,
			},
		},
		{
			name:            "unchanged — no status update",
			rootCertificate: expectedRootCert,
			httpStatus:      http.StatusOK,
			httpBody:        testTrustBundleJSON,
			want: want{
				result:          testAction.Continue(),
				rootCertificate: expectedRootCert,
			},
		},
		{
			name:       "API error — requeue",
			httpStatus: http.StatusInternalServerError,
			httpBody:   "error",
			want: want{
				result:          &action.Result{Result: reconcile.Result{RequeueAfter: 5 * time.Second}},
				rootCertificate: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			const baseURL = "http://fulcio-server.default.svc"

			httpmock.StubClientBuilder(t, baseURL+"/api/v2/trustBundle", tt.httpStatus, tt.httpBody)

			instance := &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "default"},
				Status: rhtasv1.FulcioStatus{
					Url:              baseURL,
					CertificateChain: tt.rootCertificate,
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Initialize.String()},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).Build()

			a := testAction.PrepareAction(c, NewResolvePubKeyAction())
			got := a.Handle(ctx, instance)

			g.Expect(got).To(Equal(tt.want.result))
			g.Expect(instance.Status.CertificateChain).To(Equal(tt.want.rootCertificate))
		})
	}
}
